package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func handleReload(w http.ResponseWriter, _ *http.Request) {
	if err := LoadRepos(CONF_DIR); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "OK")
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

	params := SyncRepoKeyValuesParams{
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
	if err := SyncRepoKeyValues(ctx, params); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintln(w, "OK")
}

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	repoName := strings.TrimPrefix(r.URL.Path, "/")

	// Health check endpoint (no auth required, supports any method)
	if repoName == "_healthz" {
		fmt.Fprintln(w, "OK")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	apiKey := r.Header.Get("X-API-Key")

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

func syncAllRepos(ctx context.Context) error {
	if err := LoadRepos(CONF_DIR); err != nil {
		return fmt.Errorf("failed to reload repos: %w", err)
	}

	repos := GetRepos()

	for name, repo := range repos {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		params := SyncRepoKeyValuesParams{
			Dir:          filepath.Join(DATA_DIR, name),
			URL:          repo.URL,
			Username:     repo.Username,
			Password:     repo.Password,
			Branch:       repo.Branch,
			Path:         repo.Path,
			GitUserName:  repo.GitUserName,
			GitUserEmail: repo.GitUserEmail,
			Data:         nil, // Empty data to just initialize the repo
			MaxRetries:   REPO_MAX_RETRIES,
		}
		if err := SyncRepoKeyValues(ctx, params); err != nil {
			log.Printf("Failed to initialize repo %s: %v", name, err)
		}
	}
	return nil
}

func runPeriodicRepoInit(ctx context.Context, wg *sync.WaitGroup, interval time.Duration) {
	defer wg.Done()

	// Perform initial sync immediately
	if err := syncAllRepos(ctx); err != nil {
		log.Printf("Initial repo sync failed: %v", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Periodic repo init stopped")
			return
		case <-ticker.C:
			if err := syncAllRepos(ctx); err != nil {
				log.Printf("Periodic repo sync failed: %v", err)
			}
		}
	}
}

func runServerWithLifecycle(ctx context.Context) error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: http.HandlerFunc(handleHTTP),
	}

	// Graceful shutdown handler
	go func() {
		<-ctx.Done()

		log.Println("Shutting down server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
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

	// Create root context for application lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start periodic repo initialization goroutine
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go runPeriodicRepoInit(ctx, wg, 5*time.Minute)

	// Run server in a goroutine
	serverErrChan := make(chan error, 1)
	go func() {
		serverErrChan <- runServerWithLifecycle(ctx)
	}()

	// Wait for shutdown signal or server error
	select {
	case sig := <-sigChan:
		log.Printf("Received signal: %v", sig)
	case err := <-serverErrChan:
		if err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}

	// Cancel context to signal all goroutines to stop
	cancel()

	// Wait for periodic repo init goroutine to finish
	wg.Wait()

	log.Println("Application shutdown complete")
}
