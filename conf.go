package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// RepoConf defines the configuration for a key-value repository.
type RepoConf struct {
	// APIKey is the secret key used to authenticate requests to this repository.
	APIKey string `yaml:"api_key"`

	// URL is the Git repository remote URL (e.g., https://github.com/user/repo.git).
	URL string `yaml:"url"`

	// Username is the Git authentication username.
	Username string `yaml:"username"`

	// Password is the Git authentication password or personal access token.
	Password string `yaml:"password"`

	// Branch is the Git branch to checkout and push changes to.
	Branch string `yaml:"branch"`

	// Path is the relative path within the repository to the JSON file that stores key-value data.
	// For example: "config/kv.json" or "data/settings.json".
	Path string `yaml:"path"`

	// GitUserName is the user name used for Git commits.
	GitUserName string `yaml:"git_user_name"`

	// GitUserEmail is the user email used for Git commits.
	GitUserEmail string `yaml:"git_user_email"`
}

func (c RepoConf) Validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("api_key is required")
	}
	if strings.TrimSpace(c.URL) == "" {
		return errors.New("url is required")
	}
	if strings.TrimSpace(c.Username) == "" {
		return errors.New("username is required")
	}
	if strings.TrimSpace(c.Password) == "" {
		return errors.New("password is required")
	}
	if strings.TrimSpace(c.Branch) == "" {
		return errors.New("branch is required")
	}
	if strings.TrimSpace(c.Path) == "" {
		return errors.New("path is required")
	}
	if strings.TrimSpace(c.GitUserName) == "" {
		return errors.New("git_user_name is required")
	}
	if strings.TrimSpace(c.GitUserEmail) == "" {
		return errors.New("git_user_email is required")
	}
	return nil
}

var (
	repos   map[string]RepoConf
	reposMu = &sync.RWMutex{}
)

func LoadRepos(dir string) error {
	reposMu.Lock()
	defer reposMu.Unlock()

	repos = make(map[string]RepoConf)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		// 获取文件名（不含扩展名）作为 key
		ext := filepath.Ext(name)
		key := strings.TrimSuffix(name, ext)

		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var conf RepoConf
		if err := yaml.Unmarshal(data, &conf); err != nil {
			return err
		}

		if err := conf.Validate(); err != nil {
			return fmt.Errorf("invalid config %s: %w", name, err)
		}

		repos[key] = conf
	}

	return nil
}

func GetRepos() map[string]RepoConf {
	reposMu.RLock()
	defer reposMu.RUnlock()

	out := make(map[string]RepoConf, len(repos))
	maps.Copy(out, repos)
	return out
}

func GetRepo(name string) (RepoConf, bool) {
	reposMu.RLock()
	defer reposMu.RUnlock()
	conf, ok := repos[name]
	return conf, ok
}
