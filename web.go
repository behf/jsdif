package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/behf/jsdif/watcher"
)

//go:embed web
var webFS embed.FS

func startWebServer(addr string, serverPort string, username string, password string) {
	port = serverPort // Set the global port variable
	// Create authentication middleware
	authMiddleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if username != "" && password != "" {
				user, pass, ok := r.BasicAuth()
				if !ok || user != username || pass != password {
					w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
			next.ServeHTTP(w, r)
		}
	}

	// Register API routes with auth middleware
	http.HandleFunc("/api/urls", authMiddleware(handleUrls))
	http.HandleFunc("/api/update-status", authMiddleware(handleUpdateStatus))
	http.HandleFunc("/api/add-url", authMiddleware(handleAddUrl))
	http.HandleFunc("/api/edit-url", authMiddleware(handleEditUrl))
	http.HandleFunc("/api/delete-url", authMiddleware(handleDeleteUrl))
	http.HandleFunc("/api/commits", authMiddleware(handleCommits))
	http.HandleFunc("/api/diff", authMiddleware(handleDiff))

	// Serve embedded static files
	fsys, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}

	// Create file server handler with auth
	fileServer := http.FileServer(http.FS(fsys))
	http.HandleFunc("/", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		fileServer.ServeHTTP(w, r)
	}))

	log.Fatal(http.ListenAndServe(addr, nil))
}

// isValidURL checks if the provided URL is valid and handles both website URLs and direct .js file URLs
func isValidURL(rawURL string) error {
	// Parse the URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format")
	}

	// Check scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL must start with http:// or https://")
	}

	// Check host
	if parsedURL.Host == "" {
		return fmt.Errorf("URL must include a valid domain")
	}

	// If path exists, validate it's a reasonable length and contains valid characters
	if len(parsedURL.Path) > 0 {
		// Max path length check (reasonable limit)
		if len(parsedURL.Path) > 2048 {
			return fmt.Errorf("URL path is too long")
		}
	}

	return nil
}

func handleUrls(w http.ResponseWriter, r *http.Request) {
	configs, err := watcher.LoadWatcherConfigs()
	if err != nil {
		http.Error(w, "Failed to load configurations", http.StatusInternalServerError)
		return
	}

	urlInfos := make([]struct {
		URL          string                     `json:"url"`
		Interval     int                        `json:"interval"` // in minutes
		Status       string                     `json:"status"`
		Timeout      int                        `json:"timeout"`
		Notification watcher.NotificationConfig `json:"notification"`
	}, len(configs))

	for i, config := range configs {
		urlInfos[i].URL = config.URL
		urlInfos[i].Interval = int(config.Interval.Minutes())
		urlInfos[i].Status = config.Status
		urlInfos[i].Timeout = config.Timeout
		urlInfos[i].Notification = config.Notification
	}

	sort.Slice(urlInfos, func(i, j int) bool {
		return urlInfos[i].URL < urlInfos[j].URL
	})

	json.NewEncoder(w).Encode(urlInfos)
}

func handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	url := normalizeURL(r.URL.Query().Get("url"))
	status := r.URL.Query().Get("status")

	if url == "" || status == "" {
		http.Error(w, "URL and status parameters required", http.StatusBadRequest)
		return
	}

	configs, err := watcher.LoadWatcherConfigs()
	if err != nil {
		http.Error(w, "Failed to load configurations", http.StatusInternalServerError)
		return
	}

	found := false
	for i, c := range configs {
		if c.URL == url {
			configs[i].Status = status
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	if err := watcher.SaveWatcherConfigs(configs); err != nil {
		http.Error(w, "Failed to save configurations", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	url := normalizeURL(r.URL.Query().Get("url"))
	commitHash := r.URL.Query().Get("commit")
	if url == "" || commitHash == "" {
		http.Error(w, "URL and commit parameters required", http.StatusBadRequest)
		return
	}

	watchersMutex.Lock()
	jsw, exists := activeWatchers[url]
	watchersMutex.Unlock()

	if !exists {
		http.Error(w, "Watcher not found for URL", http.StatusNotFound)
		return
	}

	diffResult, err := GetDiff(jsw.gitRepoDir, commitHash)
	if err != nil {
		http.Error(w, "Failed to get diff: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(diffResult.Content))
}

func handleDeleteUrl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	url := normalizeURL(r.URL.Query().Get("url"))
	if url == "" {
		http.Error(w, "URL parameter required", http.StatusBadRequest)
		return
	}

	configs, err := watcher.LoadWatcherConfigs()
	if err != nil {
		http.Error(w, "Failed to load configurations", http.StatusInternalServerError)
		return
	}

	found := false
	newConfigs := make([]watcher.WatcherConfig, 0, len(configs))
	for _, c := range configs {
		if c.URL == url {
			found = true
			continue
		}
		newConfigs = append(newConfigs, c)
	}

	if !found {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	if err := watcher.SaveWatcherConfigs(newConfigs); err != nil {
		http.Error(w, "Failed to save configurations", http.StatusInternalServerError)
		return
	}

	watchersMutex.Lock()
	if watcher, exists := activeWatchers[url]; exists {
		watcher.Stop()
		delete(activeWatchers, url)
	}
	watchersMutex.Unlock()

	w.WriteHeader(http.StatusOK)
}

func handleCommits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	url := normalizeURL(r.URL.Query().Get("url"))
	if url == "" {
		http.Error(w, "URL parameter required", http.StatusBadRequest)
		return
	}

	watchersMutex.Lock()
	jsw, exists := activeWatchers[url]
	watchersMutex.Unlock()

	if !exists {
		http.Error(w, "Watcher not found for URL", http.StatusNotFound)
		return
	}

	commits, err := GetCommits(jsw.gitRepoDir)
	if err != nil {
		http.Error(w, "Failed to get commits: "+err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"commits": commits,
		"total":   len(commits),
	})
}

func handleAddUrl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var rawConfig struct {
		URL          string                     `json:"url"`
		Interval     int                        `json:"interval"` // receive as minutes
		Status       string                     `json:"status"`
		Timeout      int                        `json:"timeout"`
		Notification watcher.NotificationConfig `json:"notification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&rawConfig); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Normalize URL by removing trailing slashes
	normalizedURL := normalizeURL(rawConfig.URL)

	if err := isValidURL(normalizedURL); err != nil {
		http.Error(w, fmt.Sprintf("Invalid URL: %s", err.Error()), http.StatusBadRequest)
		return
	}

	config := watcher.WatcherConfig{
		URL:          normalizedURL,
		Interval:     time.Duration(rawConfig.Interval) * time.Minute,
		Status:       rawConfig.Status,
		Timeout:      rawConfig.Timeout,
		Notification: rawConfig.Notification,
	}

	isDuplicate, err := watcher.IsURLDuplicate(config.URL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to check for duplicate URLs: %v", err), http.StatusInternalServerError)
		return
	}

	if isDuplicate {
		http.Error(w, "This URL is already being monitored. Please use a different URL or edit the existing entry.", http.StatusConflict)
		return
	}

	configs, err := watcher.LoadWatcherConfigs()
	if err != nil {
		http.Error(w, "Failed to load configurations", http.StatusInternalServerError)
		return
	}

	if config.Status == "" {
		config.Status = "active"
	}

	configs = append(configs, config)
	if err := watcher.SaveWatcherConfigs(configs); err != nil {
		http.Error(w, "Failed to save configurations", http.StatusInternalServerError)
		return
	}

	if config.Status == "active" {
		jsw := NewJsWatcher(config.URL, config.Interval, port)
		jsw.timeout = config.Timeout
		jsw.status = config.Status
		activeWatchers[config.URL] = jsw

		go jsw.Start()

		go func() {
			jsFiles, err := jsw.fetchJsFiles()
			if err != nil {
				log.Printf("Error in initial fetch for %s: %v", config.URL, err)
				return
			}
			if len(jsFiles) > 0 {
				if err := jsw.saveAndCommit(jsFiles); err != nil {
					log.Printf("Error in initial commit for %s: %v", config.URL, err)
				}
			}
		}()

		if config.Notification.Enabled && config.Notification.Type == "telegram" {
			message := fmt.Sprintf(
				"<b>🔍 New URL Monitoring Started</b>\n\nURL: %s\nInterval: %s\nTimeout: %d seconds",
				config.URL,
				formatInterval(config.Interval),
				config.Timeout,
			)
			if err := watcher.SendTelegramNotification(config.Notification.Token, config.Notification.ChatID, message); err != nil {
				log.Printf("Failed to send Telegram notification: %v", err)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func handleEditUrl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var rawConfig struct {
		URL          string                     `json:"url"`
		Interval     int                        `json:"interval"` // receive as minutes
		Status       string                     `json:"status"`
		Timeout      int                        `json:"timeout"`
		Notification watcher.NotificationConfig `json:"notification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&rawConfig); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Normalize URL by removing trailing slashes
	normalizedURL := normalizeURL(rawConfig.URL)

	if err := isValidURL(normalizedURL); err != nil {
		http.Error(w, fmt.Sprintf("Invalid URL: %s", err.Error()), http.StatusBadRequest)
		return
	}

	newConfig := watcher.WatcherConfig{
		URL:          normalizedURL,
		Interval:     time.Duration(rawConfig.Interval) * time.Minute,
		Status:       rawConfig.Status,
		Timeout:      rawConfig.Timeout,
		Notification: rawConfig.Notification,
	}

	configs, err := watcher.LoadWatcherConfigs()
	if err != nil {
		http.Error(w, "Failed to load configurations", http.StatusInternalServerError)
		return
	}

	found := false
	for i, c := range configs {
		if c.URL == newConfig.URL {
			configs[i] = newConfig
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	if err := watcher.SaveWatcherConfigs(configs); err != nil {
		http.Error(w, "Failed to save configurations", http.StatusInternalServerError)
		return
	}

	watchersMutex.Lock()
	if watcher, exists := activeWatchers[newConfig.URL]; exists {
		watcher.Stop()
		delete(activeWatchers, newConfig.URL)
		if newConfig.Status == "active" {
			jsw := NewJsWatcher(newConfig.URL, newConfig.Interval, port)
			jsw.timeout = newConfig.Timeout
			jsw.status = newConfig.Status
			activeWatchers[newConfig.URL] = jsw
			go jsw.Start()
		}
	}
	watchersMutex.Unlock()

	w.WriteHeader(http.StatusOK)
}
