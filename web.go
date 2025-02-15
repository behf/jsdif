package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Commit struct {
	Hash string    `json:"hash"`
	Date time.Time `json:"date"`
}

type CommitsResponse struct {
	Commits []Commit `json:"commits"`
	Total   int      `json:"total"`
}

type NotificationConfig struct {
	Type    string `json:"type"`    // "telegram"
	Token   string `json:"token"`   // Bot token
	ChatID  string `json:"chat_id"` // Chat ID or username
	Enabled bool   `json:"enabled"`
}

type WatcherConfig struct {
	URL          string             `json:"url"`
	Interval     time.Duration      `json:"interval"` // stored as time.Duration but received as seconds
	Status       string             `json:"status"`   // "active" or "disabled"
	Timeout      int                `json:"timeout"`  // timeout in seconds
	Notification NotificationConfig `json:"notification"`
}

func sendTelegramNotification(token string, chatID string, message string) error {
	baseURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	data := map[string]string{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "HTML",
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	resp, err := http.Post(baseURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API error: %s", string(body))
	}
	return nil
}

var (
	activeWatchers = make(map[string]*JsWatcher)
	watchersMutex  sync.RWMutex
)

func loadWatcherConfigs() ([]WatcherConfig, error) {
	data, err := os.ReadFile("watchers.json")
	if os.IsNotExist(err) {
		return []WatcherConfig{}, nil
	}
	if err != nil {
		return nil, err
	}

	var configs []WatcherConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, err
	}
	return configs, nil
}

func saveWatcherConfigs(configs []WatcherConfig) error {
	data, err := json.MarshalIndent(configs, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile("watchers.json", data, 0644)
}

func startWebServer(addr string) {
	// Load and start saved watchers
	configs, err := loadWatcherConfigs()
	if err != nil {
		log.Printf("Error loading watcher configs: %v", err)
	} else {
		for _, config := range configs {
			if config.Status == "active" {
				startWatcher(config.URL, config.Interval)
			}
		}
	}

	// Serve static files
	fs := http.FileServer(http.Dir("web"))
	http.Handle("/", fs)

	// API endpoints
	http.HandleFunc("/api/urls", handleUrls)
	http.HandleFunc("/api/commits", handleCommits)
	http.HandleFunc("/api/diff", handleDiff)
	http.HandleFunc("/api/add-url", handleAddUrl)
	http.HandleFunc("/api/delete-url", handleDeleteUrl)
	http.HandleFunc("/api/edit-url", handleEditUrl)
	http.HandleFunc("/api/update-status", handleUpdateStatus)

	log.Printf("Starting web server at %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Web server error: %v", err)
	}
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

	configs, err := loadWatcherConfigs()
	if err != nil {
		http.Error(w, "Failed to load configurations", http.StatusInternalServerError)
		return
	}

	// Find and remove the URL from configs
	found := false
	newConfigs := make([]WatcherConfig, 0)
	for _, c := range configs {
		if c.URL != url {
			newConfigs = append(newConfigs, c)
		} else {
			found = true
		}
	}

	if !found {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	if err := saveWatcherConfigs(newConfigs); err != nil {
		http.Error(w, "Failed to save configurations", http.StatusInternalServerError)
		return
	}

	// Stop and remove the watcher
	watchersMutex.Lock()
	if watcher, exists := activeWatchers[url]; exists {
		watcher.Stop() // Stop the watcher gracefully
		delete(activeWatchers, url)
	}
	watchersMutex.Unlock()

	// Remove git repository folder
	dirName := strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://"), "/", "_")
	repoPath := filepath.Join("js_snapshots", dirName)
	if err := os.RemoveAll(repoPath); err != nil {
		log.Printf("Error removing repository directory %s: %v", repoPath, err)
	}

	w.WriteHeader(http.StatusOK)
}

func handleEditUrl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var rawConfig struct {
		URL          string             `json:"url"`
		Interval     int                `json:"interval"` // receive as seconds
		Status       string             `json:"status"`
		Timeout      int                `json:"timeout"`
		Notification NotificationConfig `json:"notification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&rawConfig); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Convert seconds to time.Duration
	newConfig := WatcherConfig{
		URL:          rawConfig.URL,
		Interval:     time.Duration(rawConfig.Interval) * time.Second,
		Status:       rawConfig.Status,
		Timeout:      rawConfig.Timeout,
		Notification: rawConfig.Notification,
	}

	configs, err := loadWatcherConfigs()
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

	if err := saveWatcherConfigs(configs); err != nil {
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

func handleAddUrl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var rawConfig struct {
		URL          string             `json:"url"`
		Interval     int                `json:"interval"` // receive as seconds
		Status       string             `json:"status"`
		Timeout      int                `json:"timeout"`
		Notification NotificationConfig `json:"notification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&rawConfig); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Convert seconds to time.Duration
	config := WatcherConfig{
		URL:          rawConfig.URL,
		Interval:     time.Duration(rawConfig.Interval) * time.Second,
		Status:       rawConfig.Status,
		Timeout:      rawConfig.Timeout,
		Notification: rawConfig.Notification,
	}

	// Add new watcher config
	configs, err := loadWatcherConfigs()
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
	if err := saveWatcherConfigs(configs); err != nil {
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
			if err := sendTelegramNotification(config.Notification.Token, config.Notification.ChatID, message); err != nil {
				log.Printf("Failed to send Telegram notification: %v", err)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func formatInterval(d time.Duration) string {
	seconds := int(d.Seconds())
	switch seconds {
	case 60:
		return "Every Minute"
	case 3600:
		return "Hourly"
	case 86400:
		return "Daily"
	case 604800:
		return "Weekly"
	default:
		return fmt.Sprintf("%d seconds", seconds)
	}
}

func startWatcher(url string, interval time.Duration) {
	watchersMutex.Lock()
	defer watchersMutex.Unlock()

	configs, err := loadWatcherConfigs()
	if err != nil {
		log.Printf("Error loading watcher configs: %v", err)
		return
	}

	// Find the config for this URL to get timeout and status
	var timeout int
	var status string
	for _, c := range configs {
		if c.URL == url {
			timeout = c.Timeout
			status = c.Status
			break
		}
	}

	if _, exists := activeWatchers[url]; !exists && status == "active" {
		watcher := NewJsWatcher(url, interval)
		watcher.timeout = timeout
		watcher.status = status
		activeWatchers[url] = watcher
		go watcher.Start()
	}
}

// URLInfo represents the information about a URL that is sent to clients
type URLInfo struct {
	URL          string             `json:"url"`
	Status       string             `json:"status"`
	Interval     int                `json:"interval"` // send as seconds
	Timeout      int                `json:"timeout"`
	Notification NotificationConfig `json:"notification"`
}

func handleUrls(w http.ResponseWriter, r *http.Request) {
	configs, err := loadWatcherConfigs()
	if err != nil {
		http.Error(w, "Failed to load configurations", http.StatusInternalServerError)
		return
	}

	urlInfos := make([]URLInfo, len(configs))
	for i, config := range configs {
		urlInfos[i] = URLInfo{
			URL:          config.URL,
			Status:       config.Status,
			Interval:     int(config.Interval.Seconds()),
			Timeout:      config.Timeout,
			Notification: config.Notification,
		}
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

	configs, err := loadWatcherConfigs()
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

	if err := saveWatcherConfigs(configs); err != nil {
		http.Error(w, "Failed to save configurations", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleCommits(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		http.Error(w, "URL parameter required", http.StatusBadRequest)
		return
	}

	start := 0
	if s := r.URL.Query().Get("start"); s != "" {
		start, _ = strconv.Atoi(s)
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}

	// Convert URL to directory name
	dirName := strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://"), "/", "_")
	repoPath := filepath.Join("js_snapshots", dirName)

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		http.Error(w, "Failed to open repository", http.StatusInternalServerError)
		return
	}

	ref, err := repo.Head()
	if err != nil {
		http.Error(w, "Failed to get HEAD reference", http.StatusInternalServerError)
		return
	}

	cIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		http.Error(w, "Failed to get commit history", http.StatusInternalServerError)
		return
	}

	var commits []Commit
	err = cIter.ForEach(func(c *object.Commit) error {
		commits = append(commits, Commit{
			Hash: c.Hash.String(),
			Date: c.Author.When,
		})
		return nil
	})

	if err != nil {
		http.Error(w, "Failed to iterate commits", http.StatusInternalServerError)
		return
	}

	// Sort commits by date (newest first)
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].Date.After(commits[j].Date)
	})

	// Paginate results
	total := len(commits)
	end := start + limit
	if end > total {
		end = total
	}
	if start < total {
		commits = commits[start:end]
	} else {
		commits = []Commit{}
	}

	response := CommitsResponse{
		Commits: commits,
		Total:   total,
	}

	json.NewEncoder(w).Encode(response)
}

func handleDiff(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	commitHash := r.URL.Query().Get("commit")
	if url == "" || commitHash == "" {
		http.Error(w, "URL and commit parameters required", http.StatusBadRequest)
		return
	}

	// Convert URL to directory name
	dirName := strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://"), "/", "_")
	repoPath := filepath.Join("js_snapshots", dirName)

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		http.Error(w, "Failed to open repository", http.StatusInternalServerError)
		return
	}

	hash := plumbing.NewHash(commitHash)
	commit, err := repo.CommitObject(hash)
	if err != nil {
		http.Error(w, "Failed to get commit", http.StatusInternalServerError)
		return
	}

	parent, err := commit.Parent(0)
	if err != nil {
		// If no parent (first commit), return empty diff
		fmt.Fprint(w, "")
		return
	}

	parentTree, err := parent.Tree()
	if err != nil {
		http.Error(w, "Failed to get parent tree", http.StatusInternalServerError)
		return
	}

	commitTree, err := commit.Tree()
	if err != nil {
		http.Error(w, "Failed to get commit tree", http.StatusInternalServerError)
		return
	}

	patch, err := parentTree.Patch(commitTree)
	if err != nil {
		http.Error(w, "Failed to create patch", http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, patch.String())
}
