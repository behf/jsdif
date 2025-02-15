package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/watchtover-gitdif/watcher"
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

func startWebServer(addr string) {
	http.HandleFunc("/api/urls", handleUrls)
	http.HandleFunc("/api/update-status", handleUpdateStatus)
	http.HandleFunc("/api/add-url", handleAddUrl)
	http.HandleFunc("/api/edit-url", handleEditUrl)

	// Serve static files
	http.Handle("/", http.FileServer(http.Dir("web")))

	log.Printf("Starting web server on %s", addr)
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
	watcher := NewJsWatcher(url, interval)
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

	// Start the new watcher if status is active
	if config.Status == "active" {
		startWatcher(config.URL, config.Interval)

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
			watcher := NewJsWatcher(newConfig.URL, newConfig.Interval)
			watcher.timeout = newConfig.Timeout
			watcher.status = newConfig.Status
			activeWatchers[newConfig.URL] = watcher
			go watcher.Start()
		}
	}
	watchersMutex.Unlock()

	w.WriteHeader(http.StatusOK)
}
