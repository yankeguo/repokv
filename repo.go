package main

import (
	"context"
	"encoding/json"
	"fmt"
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

// UpdateRepoKeyValueParams contains all parameters needed to update key-value data in a Git repository.
type UpdateRepoKeyValueParams struct {
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
	Data map[string]string

	// MaxRetries is the maximum number of retry attempts for failed operations.
	MaxRetries int
}

func UpdateRepoKeyValue(ctx context.Context, params UpdateRepoKeyValueParams) error {
	if params.MaxRetries <= 0 {
		params.MaxRetries = 3
	}

	// 获取目录锁，确保同一目录串行操作
	lock := getDirLock(params.Dir)
	lock.Lock()
	defer lock.Unlock()

	var lastErr error
	for i := 0; i < params.MaxRetries; i++ {
		if i > 0 {
			time.Sleep(time.Second * time.Duration(i))
		}

		if err := updateRepoKeyValueOnce(ctx, params); err != nil {
			lastErr = err
			continue
		}
		return nil
	}

	return fmt.Errorf("failed after %d retries: %w", params.MaxRetries, lastErr)
}

func updateRepoKeyValueOnce(ctx context.Context, params UpdateRepoKeyValueParams) error {
	// 检查目录是否存在且是 git 仓库
	gitDir := filepath.Join(params.Dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// 目录不存在或不是 git 仓库，需要克隆
		if err := os.RemoveAll(params.Dir); err != nil {
			return fmt.Errorf("failed to remove old dir: %w", err)
		}
		if err := os.MkdirAll(params.Dir, 0755); err != nil {
			return fmt.Errorf("failed to create dir: %w", err)
		}

		// 克隆仓库
		cloneURL := params.URL
		if params.Username != "" && params.Password != "" {
			cloneURL = insertCredentials(params.URL, params.Username, params.Password)
		}

		cmd := exec.CommandContext(ctx, "git", "clone", "-b", params.Branch, "--single-branch", cloneURL, params.Dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git clone failed: %w, output: %s", err, string(out))
		}
	} else if err != nil {
		return fmt.Errorf("failed to check git dir: %w", err)
	} else {
		// 是 git 仓库，更新远端URL、清理并拉取最新代码
		// 准备带认证的URL
		remoteURL := params.URL
		if params.Username != "" && params.Password != "" {
			remoteURL = insertCredentials(params.URL, params.Username, params.Password)
		}

		// 检查 origin 是否存在，不存在则添加，存在则更新URL
		cmd := exec.CommandContext(ctx, "git", "-C", params.Dir, "remote", "get-url", "origin")
		if err := cmd.Run(); err != nil {
			// origin 不存在，添加 origin
			cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "remote", "add", "origin", remoteURL)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("git remote add failed: %w, output: %s", err, string(out))
			}
		} else {
			// origin 存在，更新URL
			cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "remote", "set-url", "origin", remoteURL)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("git remote set-url failed: %w, output: %s", err, string(out))
			}
		}

		// 清理未跟踪的文件和目录
		cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "clean", "-fd")
		_ = cmd.Run() // 忽略错误，继续执行

		// 重置所有变更
		cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "reset", "--hard")
		_ = cmd.Run() // 忽略错误，继续执行

		// 获取远端最新
		cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "fetch", "origin", params.Branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git fetch failed: %w, output: %s", err, string(out))
		}

		// 强制重置到远端分支
		cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "reset", "--hard", "origin/"+params.Branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git reset failed: %w, output: %s", err, string(out))
		}
	}

	// 确保在正确的分支（可能是分离头指针，需要创建/切换到本地分支）
	cmd := exec.CommandContext(ctx, "git", "-C", params.Dir, "checkout", "-B", params.Branch, "origin/"+params.Branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout failed: %w, output: %s", err, string(out))
	}

	// 更新 JSON 文件
	filePath := filepath.Join(params.Dir, params.Path)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
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
		return fmt.Errorf("failed to marshal json: %w", err)
	}

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// 检查是否有变更
	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "diff", "--quiet")
	if err := cmd.Run(); err == nil {
		// 没有变更
		return nil
	}

	// 配置 git 用户信息
	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "config", "user.email", params.GitUserEmail)
	_ = cmd.Run()
	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "config", "user.name", params.GitUserName)
	_ = cmd.Run()

	// 提交变更
	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "add", params.Path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w, output: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "commit", "-m", "update key-value")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %w, output: %s", err, string(out))
	}

	// 推送
	pushURL := params.URL
	if params.Username != "" && params.Password != "" {
		pushURL = insertCredentials(params.URL, params.Username, params.Password)
	}

	cmd = exec.CommandContext(ctx, "git", "-C", params.Dir, "push", pushURL, params.Branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push failed: %w, output: %s", err, string(out))
	}

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
