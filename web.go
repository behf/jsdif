package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/mirzaaghazadeh/jsdif/watcher"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var (
	activeWatchers = make(map[string]*JsWatcher)
	watchersMutex  sync.Mutex
)

func formatInterval(d time.Duration) string {
	// Convert to minutes for better readability
	minutes := int(d.Minutes())
	if minutes < 1 {
		seconds := int(d.Seconds())
		return fmt.Sprintf("%ds", seconds)
	} else if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	} else {
		hours := minutes / 60
		remainingMinutes := minutes % 60
		if remainingMinutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, remainingMinutes)
	}
}

var serverPort string

func startWebServer(addr string, port string) {
	serverPort = port
	http.HandleFunc("/api/urls", handleUrls)
	http.HandleFunc("/api/update-status", handleUpdateStatus)
	http.HandleFunc("/api/add-url", handleAddUrl)
	http.HandleFunc("/api/edit-url", handleEditUrl)
	http.HandleFunc("/api/delete-url", handleDeleteUrl)
	http.HandleFunc("/api/commits", handleCommits)
	http.HandleFunc("/api/diff", handleDiff)

	// Serve static files from the web directory using OS-agnostic path
	webDir := http.Dir(filepath.Join(".", "web"))
	http.Handle("/", http.FileServer(webDir))

	// log.Printf("%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func startWatcher(url string, interval time.Duration) {
	watchersMutex.Lock()
	defer watchersMutex.Unlock()

	// Stop any existing watcher for this URL
	if existingWatcher, exists := activeWatchers[url]; exists {
		existingWatcher.Stop()
	}

	// Create and start new watcher
	watcher := NewJsWatcher(url, interval, serverPort)
	activeWatchers[url] = watcher
	go watcher.Start()
}

func handleUrls(w http.ResponseWriter, r *http.Request) {
	configs, err := watcher.LoadWatcherConfigs()
	if err != nil {
		http.Error(w, "Failed to load configurations", http.StatusInternalServerError)
		return
	}

	// Convert interval to minutes for display
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

	// Sort by URL
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

	url := r.URL.Query().Get("url")
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

	// Find and update the URL config
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

	url := r.URL.Query().Get("url")
	commitHash := r.URL.Query().Get("commit")
	if url == "" || commitHash == "" {
		http.Error(w, "URL and commit parameters required", http.StatusBadRequest)
		return
	}

	// Find the watcher for this URL
	watchersMutex.Lock()
	jsw, exists := activeWatchers[url]
	watchersMutex.Unlock()

	if !exists {
		http.Error(w, "Watcher not found for URL", http.StatusNotFound)
		return
	}

	repo, err := git.PlainOpen(jsw.gitRepoDir)
	if err != nil {
		http.Error(w, "Failed to open git repository", http.StatusInternalServerError)
		return
	}

	// Get commit object
	hash := plumbing.NewHash(commitHash)
	commit, err := repo.CommitObject(hash)
	if err != nil {
		http.Error(w, "Failed to get commit", http.StatusInternalServerError)
		return
	}

	// Get parent commit
	parent, err := commit.Parent(0)
	if err != nil && err != object.ErrParentNotFound {
		http.Error(w, "Failed to get parent commit", http.StatusInternalServerError)
		return
	}

	// Get commit trees
	currentTree, err := commit.Tree()
	if err != nil {
		http.Error(w, "Failed to get commit tree", http.StatusInternalServerError)
		return
	}

	var patch *object.Patch
	if parent == nil {
		// For first commit, create empty tree for comparison
		emptyTree := &object.Tree{}
		patch, err = currentTree.Patch(emptyTree)
	} else {
		parentTree, err := parent.Tree()
		if err != nil {
			http.Error(w, "Failed to get parent tree", http.StatusInternalServerError)
			return
		}
		patch, err = parentTree.Patch(currentTree)
	}

	if err != nil {
		http.Error(w, "Failed to get diff", http.StatusInternalServerError)
		return
	}

	// Return the unified diff format
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(patch.String()))
}

func handleDeleteUrl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	url := r.URL.Query().Get("url")
	if url == "" {
		http.Error(w, "URL parameter required", http.StatusBadRequest)
		return
	}

	configs, err := watcher.LoadWatcherConfigs()
	if err != nil {
		http.Error(w, "Failed to load configurations", http.StatusInternalServerError)
		return
	}

	// Find and remove the URL config
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

	// Stop and remove watcher if active
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

	url := r.URL.Query().Get("url")
	if url == "" {
		http.Error(w, "URL parameter required", http.StatusBadRequest)
		return
	}

	// Find the watcher for this URL
	watchersMutex.Lock()
	jsw, exists := activeWatchers[url]
	watchersMutex.Unlock()

	if !exists {
		http.Error(w, "Watcher not found for URL", http.StatusNotFound)
		return
	}

	repo, err := git.PlainOpen(jsw.gitRepoDir)
	if err != nil {
		if err == git.ErrRepositoryNotExists {
			// Return empty commits list if repo doesn't exist yet
			json.NewEncoder(w).Encode(map[string]interface{}{
				"commits": []interface{}{},
				"total":   0,
			})
			return
		}
		http.Error(w, "Failed to open git repository", http.StatusInternalServerError)
		return
	}

	// Get commit history
	commits, err := repo.Log(&git.LogOptions{})
	if err != nil {
		http.Error(w, "Failed to get commit history", http.StatusInternalServerError)
		return
	}

	var commitsList []map[string]interface{}
	err = commits.ForEach(func(c *object.Commit) error {
		commitsList = append(commitsList, map[string]interface{}{
			"hash": c.Hash.String(),
			"date": c.Author.When,
		})
		return nil
	})

	if err != nil {
		http.Error(w, "Failed to process commits", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"commits": commitsList,
		"total":   len(commitsList),
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

	// Convert minutes to time.Duration
	config := watcher.WatcherConfig{
		URL:          rawConfig.URL,
		Interval:     time.Duration(rawConfig.Interval) * time.Minute, // Convert minutes to Duration
		Status:       rawConfig.Status,
		Timeout:      rawConfig.Timeout,
		Notification: rawConfig.Notification,
	}

	// Add new watcher config
	configs, err := watcher.LoadWatcherConfigs()
	if err != nil {
		http.Error(w, "Failed to load configurations", http.StatusInternalServerError)
		return
	}

	// Check if URL already exists
	for _, c := range configs {
		if c.URL == config.URL {
			http.Error(w, "URL already being monitored", http.StatusBadRequest)
			return
		}
	}

	// Set default status if not provided
	if config.Status == "" {
		config.Status = "active"
	}

	configs = append(configs, config)
	if err := watcher.SaveWatcherConfigs(configs); err != nil {
		http.Error(w, "Failed to save configurations", http.StatusInternalServerError)
		return
	}

	// Create and start the new watcher if status is active
	if config.Status == "active" {
		jsw := NewJsWatcher(config.URL, config.Interval, serverPort)
		jsw.timeout = config.Timeout
		jsw.status = config.Status
		activeWatchers[config.URL] = jsw

		// Start regular monitoring
		go jsw.Start()

		// Perform initial check
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

		// Send initial notification if enabled
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

	// Convert minutes to time.Duration
	newConfig := watcher.WatcherConfig{
		URL:          rawConfig.URL,
		Interval:     time.Duration(rawConfig.Interval) * time.Minute, // Convert minutes to Duration
		Status:       rawConfig.Status,
		Timeout:      rawConfig.Timeout,
		Notification: rawConfig.Notification,
	}

	configs, err := watcher.LoadWatcherConfigs()
	if err != nil {
		http.Error(w, "Failed to load configurations", http.StatusInternalServerError)
		return
	}

	// Find and update the URL config
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

	// Update the watcher
	watchersMutex.Lock()
	if watcher, exists := activeWatchers[newConfig.URL]; exists {
		watcher.Stop() // Stop existing watcher gracefully
		delete(activeWatchers, newConfig.URL)
		// Start new watcher with updated config if status is active
		if newConfig.Status == "active" {
			jsw := NewJsWatcher(newConfig.URL, newConfig.Interval, serverPort)
			jsw.timeout = newConfig.Timeout
			jsw.status = newConfig.Status
			activeWatchers[newConfig.URL] = jsw
			go jsw.Start()
		}
	}
	watchersMutex.Unlock()

	w.WriteHeader(http.StatusOK)
}
