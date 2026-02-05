package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

var (
	dirLocks               = make(map[string]sync.Locker)
	dirLocksMu sync.Locker = &sync.Mutex{}
)

func getDirLock(dir string) sync.Locker {
	dirLocksMu.Lock()
	defer dirLocksMu.Unlock()

	if lock, ok := dirLocks[dir]; ok {
		return lock
	}

	lock := &sync.Mutex{}
	dirLocks[dir] = lock
	return lock
}

// SyncRepoKeyValuesParams contains all parameters needed to synchronize key-value data in a Git repository.
// An empty Data field can be used to initialize the repository and checkout the branch without making changes.
type SyncRepoKeyValuesParams struct {
	// Dir is the local directory path where the Git repository will be cloned.
	// This serves as the workspace for all Git operations.
	Dir string

	// URL is the Git repository remote URL.
	URL string

	// Username is the Git authentication username.
	Username string

	// Password is the Git authentication password or token.
	Password string

	// Branch is the Git branch to checkout and push to.
	Branch string

	// Path is the relative path within the repository to the JSON file storing key-value data.
	// This file will be created or updated within the cloned repository.
	Path string

	// GitUserName is the author name for Git commits.
	GitUserName string

	// GitUserEmail is the author email for Git commits.
	GitUserEmail string

	// Data contains the key-value pairs to write to the JSON file.
	// If empty, the function will only initialize the repository and checkout the branch without modifying any files.
	Data map[string]string

	// MaxRetries is the maximum number of retry attempts for failed operations.
	MaxRetries int
}

// SyncRepoKeyValues synchronizes key-value data in a Git repository.
// It clones or pulls the repository, checks out the specified branch, and updates the JSON file with the provided data.
// If Data is empty, it only initializes the repository and checks out the branch without making any file changes.
func SyncRepoKeyValues(ctx context.Context, params SyncRepoKeyValuesParams) error {
	slog.Info("start syncing repo key-values", "dir", params.Dir, "branch", params.Branch, "path", params.Path)

	if params.MaxRetries <= 0 {
		params.MaxRetries = 3
	}

	// 获取目录锁，确保同一目录串行操作
	lock := getDirLock(params.Dir)
	lock.Lock()
	defer lock.Unlock()

	var lastErr error
	for i := 0; i < params.MaxRetries; i++ {
		slog.Info("retry attempt", "attempt", i+1, "maxRetries", params.MaxRetries)

		if i > 0 {
			time.Sleep(time.Second * time.Duration(i))
		}

		if err := syncRepoKeyValuesOnce(ctx, params); err != nil {
			lastErr = err
			slog.Error("attempt failed", "attempt", i+1, "error", err)
			continue
		}
		slog.Info("syncing repo key-values succeeded")
		return nil
	}

	slog.Error("syncing repo key-values failed after retries", "maxRetries", params.MaxRetries, "error", lastErr)
	return fmt.Errorf("failed after %d retries: %w", params.MaxRetries, lastErr)
}

func syncRepoKeyValuesOnce(ctx context.Context, params SyncRepoKeyValuesParams) error {
	logger := slog.With("dir", params.Dir, "branch", params.Branch, "path", params.Path)

	logger.Info("start syncing repo key-values once")

	// 检查目录是否存在且是 git 仓库
	gitDir := filepath.Join(params.Dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		logger.Info("git dir not exist, cloning")

		// 目录不存在或不是 git 仓库，需要克隆
		if err := os.RemoveAll(params.Dir); err != nil {
			logger.Error("failed to remove old dir", "error", err)
			return fmt.Errorf("failed to remove old dir: %w", err)
		}
		if err := os.MkdirAll(params.Dir, 0755); err != nil {
			logger.Error("failed to create dir", "error", err)
			return fmt.Errorf("failed to create dir: %w", err)
		}

		// 克隆仓库
		cloneURL := params.URL
		if params.Username != "" && params.Password != "" {
			cloneURL = insertCredentials(params.URL, params.Username, params.Password)
		}

		logger.Info("git clone")
		cmd := exec.CommandContext(ctx, "git", "clone", "-b", params.Branch, "--single-branch", cloneURL, params.Dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			logger.Error("git clone failed", "error", err, "output", string(out))
			return fmt.Errorf("git clone failed: %w, output: %s", err, string(out))
		}
		logger.Info("git clone succeeded")
	} else if err != nil {
		logger.Error("failed to check git dir", "gitDir", gitDir, "error", err)
		return fmt.Errorf("failed to check git dir: %w", err)
	} else {
		logger.Info("git dir exists, updating repo")

		// 是 git 仓库，更新远端URL、清理并拉取最新代码
		// 准备带认证的URL
		remoteURL := params.URL
		if params.Username != "" && params.Password != "" {
			remoteURL = insertCredentials(params.URL, params.Username, params.Password)
		}

		// 检查 origin 是否存在，不存在则添加，存在则更新URL
		cmd := exec.CommandContext(ctx, "git", "-C", params.Dir, "remote", "get-url", "origin")
		if err := cmd.Run(); err != nil {
			logger.Info("origin not exist, adding remote")
			// origin 不存在，添加 origin
			cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "remote", "add", "origin", remoteURL)
			if out, err := cmd.CombinedOutput(); err != nil {
				logger.Error("git remote add failed", "error", err, "output", string(out))
				return fmt.Errorf("git remote add failed: %w, output: %s", err, string(out))
			}
			logger.Info("git remote add succeeded")
		} else {
			logger.Info("origin exists, updating remote url")
			// origin 存在，更新URL
			cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "remote", "set-url", "origin", remoteURL)
			if out, err := cmd.CombinedOutput(); err != nil {
				logger.Error("git remote set-url failed", "error", err, "output", string(out))
				return fmt.Errorf("git remote set-url failed: %w, output: %s", err, string(out))
			}
			logger.Info("git remote set-url succeeded")
		}

		// 清理未跟踪的文件和目录
		logger.Info("git clean")
		cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "clean", "-fd")
		_ = cmd.Run() // 忽略错误，继续执行

		// 重置所有变更
		logger.Info("git reset --hard")
		cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "reset", "--hard")
		_ = cmd.Run() // 忽略错误，继续执行

		// 获取远端最新
		logger.Info("git fetch")
		cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "fetch", "origin", params.Branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			logger.Error("git fetch failed", "error", err, "output", string(out))
			return fmt.Errorf("git fetch failed: %w, output: %s", err, string(out))
		}
		logger.Info("git fetch succeeded")

		// 强制重置到远端分支
		logger.Info("git reset to origin")
		cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "reset", "--hard", "origin/"+params.Branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			logger.Error("git reset failed", "error", err, "output", string(out))
			return fmt.Errorf("git reset failed: %w, output: %s", err, string(out))
		}
		logger.Info("git reset succeeded")
	}

	// 确保在正确的分支（可能是分离头指针，需要创建/切换到本地分支）
	logger.Info("git checkout")
	cmd := exec.CommandContext(ctx, "git", "-C", params.Dir, "checkout", "-B", params.Branch, "origin/"+params.Branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Error("git checkout failed", "error", err, "output", string(out))
		return fmt.Errorf("git checkout failed: %w, output: %s", err, string(out))
	}
	logger.Info("git checkout succeeded")

	if len(params.Data) == 0 {
		logger.Info("no data to update, skipping file operations")
		return nil
	}

	// 更新 JSON 文件
	filePath := filepath.Join(params.Dir, params.Path)
	logger.Info("updating json file", "filePath", filePath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		logger.Error("failed to create file dir", "dir", filepath.Dir(filePath), "error", err)
		return fmt.Errorf("failed to create file dir: %w", err)
	}

	var existingData map[string]any
	if content, err := os.ReadFile(filePath); err == nil {
		_ = json.Unmarshal(content, &existingData)
	}
	if existingData == nil {
		existingData = make(map[string]any)
	}

	for k, v := range params.Data {
		existingData[k] = v
	}

	content, err := json.MarshalIndent(existingData, "", "  ")
	if err != nil {
		logger.Error("failed to marshal json", "error", err)
		return fmt.Errorf("failed to marshal json: %w", err)
	}

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		logger.Error("failed to write file", "filePath", filePath, "error", err)
		return fmt.Errorf("failed to write file: %w", err)
	}
	logger.Info("json file updated")

	// 检查是否有变更
	logger.Info("checking for changes")
	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "diff", "--quiet")
	if err := cmd.Run(); err == nil {
		// 没有变更
		logger.Info("no changes detected, skipping commit")
		return nil
	}
	logger.Info("changes detected")

	// 配置 git 用户信息
	logger.Info("configuring git user")
	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "config", "user.email", params.GitUserEmail)
	_ = cmd.Run()
	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "config", "user.name", params.GitUserName)
	_ = cmd.Run()

	// 提交变更
	logger.Info("git add")
	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "add", params.Path)
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Error("git add failed", "error", err, "output", string(out))
		return fmt.Errorf("git add failed: %w, output: %s", err, string(out))
	}
	logger.Info("git add succeeded")

	logger.Info("git commit")
	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "commit", "-m", "update key-value")
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Error("git commit failed", "error", err, "output", string(out))
		return fmt.Errorf("git commit failed: %w, output: %s", err, string(out))
	}
	logger.Info("git commit succeeded")

	// 推送
	logger.Info("git push")
	pushURL := params.URL
	if params.Username != "" && params.Password != "" {
		pushURL = insertCredentials(params.URL, params.Username, params.Password)
	}

	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "push", pushURL, params.Branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Error("git push failed", "error", err, "output", string(out))
		return fmt.Errorf("git push failed: %w, output: %s", err, string(out))
	}
	logger.Info("git push succeeded")
	logger.Info("syncing repo key-values once completed successfully")

	return nil
}

func insertCredentials(rawURL, username, password string) string {
	// 使用 net/url 包解析和修改 URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.Scheme == "https" || u.Scheme == "http" {
		u.User = url.UserPassword(username, password)
	}
	return u.String()
}
