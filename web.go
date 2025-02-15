package main

import (
	"encoding/json"
	"fmt"
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

type WatcherConfig struct {
	URL      string        `json:"url"`
	Interval time.Duration `json:"interval"`
	Status   string        `json:"status"`  // "active" or "disabled"
	Timeout  int           `json:"timeout"` // timeout in seconds
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

	var newConfig WatcherConfig
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
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

	var config WatcherConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
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
	}

	w.WriteHeader(http.StatusOK)
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
	URL      string        `json:"url"`
	Status   string        `json:"status"`
	Interval time.Duration `json:"interval"`
	Timeout  int           `json:"timeout"`
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
			URL:      config.URL,
			Status:   config.Status,
			Interval: config.Interval,
			Timeout:  config.Timeout,
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
