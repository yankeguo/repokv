package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func handleReload(w http.ResponseWriter, _ *http.Request) {
	if err := LoadRepos(CONF_DIR); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte("OK"))
}

func handleRepoUpdate(w http.ResponseWriter, r *http.Request, repoName string, repo RepoConf) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data := make(map[string]string)
	for key, values := range r.PostForm {
		if len(values) > 0 {
			data[key] = values[0]
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	params := UpdateRepoKeyValueParams{
		Dir:          filepath.Join(DATA_DIR, repoName),
		URL:          repo.URL,
		Username:     repo.Username,
		Password:     repo.Password,
		Branch:       repo.Branch,
		Path:         repo.Path,
		GitUserName:  repo.GitUserName,
		GitUserEmail: repo.GitUserEmail,
		Data:         data,
		MaxRetries:   REPO_MAX_RETRIES,
	}
	if err := UpdateRepoKeyValue(ctx, params); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("OK"))
}

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	apiKey := r.Header.Get("X-API-Key")

	repoName := strings.TrimPrefix(r.URL.Path, "/")

	// Admin endpoints
	if repoName == "_reload" {
		if apiKey != ADMIN_API_KEY {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		handleReload(w, r)
		return
	}

	// Repo key-value update
	repo, found := GetRepo(repoName)
	if !found || repo.APIKey != apiKey {
		http.Error(w, "Repo not found or invalid api key", http.StatusNotFound)
		return
	}

	handleRepoUpdate(w, r, repoName, repo)
}

func runServer() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: http.HandlerFunc(handleHTTP),
	}

	// 优雅停机处理
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit

		log.Println("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Server forced to shutdown: %v", err)
		}

		log.Println("Server exited")
	}()

	log.Printf("Server starting on port %s", port)
	return server.ListenAndServe()
}

func main() {
	if err := LoadRepos(CONF_DIR); err != nil {
		log.Fatalf("Failed to load repos: %v", err)
	}

	if err := runServer(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
