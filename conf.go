package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type RepoConf struct {
	APIKey       string `yaml:"api_key"`
	URL          string `yaml:"url"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	Branch       string `yaml:"branch"`
	Path         string `yaml:"path"`
	GitUserName  string `yaml:"git_user_name"`
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
	repos     map[string]RepoConf
	reposLock = &sync.RWMutex{}
)

func LoadRepos(dir string) (err error) {
	reposLock.Lock()
	defer reposLock.Unlock()

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

func GetRepo(name string) (RepoConf, bool) {
	reposLock.RLock()
	defer reposLock.RUnlock()
	conf, ok := repos[name]
	return conf, ok
}
