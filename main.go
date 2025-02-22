package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/mirzaaghazadeh/jsdif/watcher"
)

type JsWatcher struct {
	url          string
	interval     time.Duration
	gitRepoDir   string
	timeout      int
	status       string
	stopChan     chan struct{}
	port         string
	isDirectFile bool
}

func NewJsWatcher(url string, interval time.Duration, port string) *JsWatcher {
	// Create a sanitized directory name from the URL
	dirName := strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://"), "/", "_")
	gitRepoDir := filepath.Join("js_snapshots", dirName)

	isDirectFile := strings.HasSuffix(strings.ToLower(url), ".js")

	return &JsWatcher{
		url:          url,
		interval:     interval,
		gitRepoDir:   gitRepoDir,
		status:       "active",
		timeout:      0,
		stopChan:     make(chan struct{}),
		port:         port,
		isDirectFile: isDirectFile,
	}
}

func (w *JsWatcher) fetchJsContent(jsURL string) (string, error) {
	resp, err := http.Get(jsURL)
	if err != nil {
		return "", fmt.Errorf("error fetching JS from %s: %w", jsURL, err)
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading JS content: %w", err)
	}

	return string(content), nil
}

func (w *JsWatcher) initGitRepo() (*git.Repository, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(w.gitRepoDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating directory: %w", err)
	}

	// Initialize or open git repository
	repo, err := git.PlainOpen(w.gitRepoDir)
	if err == git.ErrRepositoryNotExists {
		repo, err = git.PlainInit(w.gitRepoDir, false)
		if err != nil {
			return nil, fmt.Errorf("error initializing git repo: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("error opening git repo: %w", err)
	}

	return repo, nil
}

func (w *JsWatcher) saveAndCommit(jsFiles []string) error {
	repo, err := w.initGitRepo()
	if err != nil {
		return err
	}

	// Combine JS files without timestamp for comparison
	var jsContent strings.Builder
	for i, js := range jsFiles {
		jsContent.WriteString(fmt.Sprintf("/* JS File #%d */\n%s\n\n", i+1, js))
	}
	newContent := jsContent.String()

	// Write to file
	jsFile := filepath.Join(w.gitRepoDir, "combined.js")

	// Check if content has changed (ignoring timestamp)
	currentContent, err := os.ReadFile(jsFile)
	if err == nil {
		// Remove timestamp line from current content for comparison
		currentLines := strings.SplitN(string(currentContent), "\n\n", 2)
		if len(currentLines) > 1 && currentLines[1] == newContent {
			log.Println("No changes detected in JS files")
			return nil
		}
	}

	// Add timestamp only when writing
	var finalContent strings.Builder
	finalContent.WriteString(fmt.Sprintf("/* Last updated: %s */\n\n", time.Now().Format("2006-01-02 15:04:05")))
	finalContent.WriteString(newContent)

	if err := os.WriteFile(jsFile, []byte(finalContent.String()), 0644); err != nil {
		return fmt.Errorf("error writing combined JS: %w", err)
	}

	// Get the worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("error getting worktree: %w", err)
	}

	// Add changes
	if _, err := worktree.Add("combined.js"); err != nil {
		return fmt.Errorf("error adding file to git: %w", err)
	}

	// Check if there are actual changes to commit
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("error getting git status: %w", err)
	}

	if status.IsClean() {
		log.Println("No changes to commit")
		return nil
	}

	// Create commit
	commit, err := worktree.Commit(fmt.Sprintf("JS snapshot %s", time.Now().Format("2006-01-02 15:04:05")), &git.CommitOptions{
		Author: &object.Signature{
			Name:  "WatchTover",
			Email: "watchtover@localhost",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("error creating commit: %w", err)
	}

	log.Printf("Created commit for detected changes: %s", commit.String())

	// Send Telegram notification if enabled
	configs, err := watcher.LoadWatcherConfigs()
	if err != nil {
		log.Printf("Error loading configs for notification: %v", err)
		return nil
	}

	// Find config for this URL to get notification settings
	for _, config := range configs {
		if config.URL == w.url && config.Notification.Enabled && config.Notification.Type == "telegram" {
			message := fmt.Sprintf(
				"<b>🔔 Changes Detected!</b>\n\nURL: %s\nTimestamp: %s\nCommit: %s",
				w.url,
				time.Now().Format("2006-01-02 15:04:05"),
				commit.String()[:8],
			)
			if err := watcher.SendTelegramNotification(
				config.Notification.Token,
				config.Notification.ChatID,
				message,
			); err != nil {
				log.Printf("Error sending Telegram notification: %v", err)
			}
			break
		}
	}

	return nil
}

func (w *JsWatcher) fetchJsFiles() ([]string, error) {
	if w.isDirectFile {
		// Direct .js file URL - fetch it directly
		content, err := w.fetchJsContent(w.url)
		if err != nil {
			return nil, fmt.Errorf("error fetching direct JS file: %w", err)
		}
		return []string{content}, nil
	}

	// Regular website URL - scan for JS files
	resp, err := http.Get(w.url)
	if err != nil {
		return nil, fmt.Errorf("error fetching main page: %w", err)
	}
	defer resp.Body.Close()

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error parsing HTML: %w", err)
	}

	var jsFiles []string
	// Find all script tags
	doc.Find("script[src]").Each(func(i int, s *goquery.Selection) {
		if src, exists := s.Attr("src"); exists {
			// Handle relative URLs
			jsURL := src
			if strings.HasPrefix(src, "//") {
				jsURL = "http:" + src
			} else if !strings.HasPrefix(src, "http") {
				jsURL = fmt.Sprintf("%s/%s", strings.TrimSuffix(w.url, "/"), strings.TrimPrefix(src, "/"))
			}

			if content, err := w.fetchJsContent(jsURL); err == nil {
				jsFiles = append(jsFiles, content)
			} else {
				log.Printf("Error fetching JS from %s: %v", jsURL, err)
			}
		}
	})

	return jsFiles, nil
}

func (w *JsWatcher) Stop() {
	close(w.stopChan)
}

func (w *JsWatcher) Start() {
	log.Printf("Starting JS watcher for %s (checking every %v)", w.url, w.interval)
	log.Printf("Git repository: %s", w.gitRepoDir)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	failureCount := 0

	for {
		select {
		case <-w.stopChan:
			log.Printf("Stopping watcher for %s", w.url)
			return
		case <-ticker.C:
			log.Printf("\nChecking for JS changes at %s", time.Now().Format("2006-01-02 15:04:05"))

			jsFiles, err := w.fetchJsFiles()
			if err != nil {
				log.Printf("Error fetching JS files: %v", err)
				failureCount++
				if w.timeout > 0 && failureCount >= w.timeout {
					log.Printf("Timeout reached for %s after %d failures", w.url, failureCount)
					w.status = "disabled"
					// Update status in configuration
					client := &http.Client{}
					urlStr := fmt.Sprintf("http://localhost:%s/api/update-status?url=%s&status=disabled", w.port, url.QueryEscape(w.url))
					req, err := http.NewRequest(http.MethodPut, urlStr, nil)
					if err == nil {
						if _, err := client.Do(req); err != nil {
							log.Printf("Error updating status: %v", err)
						}
					}
					return
				}
				continue
			}

			failureCount = 0 // Reset counter on successful fetch

			if len(jsFiles) == 0 {
				log.Println("No JS files found")
				continue
			}

			if err := w.saveAndCommit(jsFiles); err != nil {
				log.Printf("Error saving and committing: %v", err)
			}
		}
	}
}

func main() {
	// Define flags
	port := "9093" // default port
	var username, password string

	// Check arguments
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("Usage: jsdif [-p PORT] [-u USERNAME] [-p PASSWORD] run")
		os.Exit(1)
	}

	// Look for flags and ensure "run" command exists
	var foundRun bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p":
			if i+1 >= len(args) || args[i+1] == "run" || strings.HasPrefix(args[i+1], "-") {
				fmt.Println("Invalid port value")
				os.Exit(1)
			}
			port = args[i+1]
			i++ // Skip the value
		case "-u":
			if i+1 >= len(args) || args[i+1] == "run" || strings.HasPrefix(args[i+1], "-") {
				fmt.Println("Invalid username value")
				os.Exit(1)
			}
			username = args[i+1]
			i++ // Skip the value
		case "--password":
			if i+1 >= len(args) || args[i+1] == "run" || strings.HasPrefix(args[i+1], "-") {
				fmt.Println("Invalid password value")
				os.Exit(1)
			}
			password = args[i+1]
			i++ // Skip the value
		case "run":
			foundRun = true
		}
	}

	if !foundRun {
		fmt.Println("Usage: jsdif [-p PORT] [-u USERNAME] [--password PASSWORD] run")
		os.Exit(1)
	}

	// Create js_snapshots directory if it doesn't exist
	if err := os.MkdirAll("js_snapshots", 0755); err != nil {
		log.Fatalf("Failed to create js_snapshots directory: %v", err)
	}

	// Debug message to ensure the correct port
	log.Printf("Starting web server on port %s", port)
	startWebServer(":"+port, port, username, password)
}
