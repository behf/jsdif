package watcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// NotificationConfig represents Telegram notification settings
type NotificationConfig struct {
	Type    string `json:"type"`    // "telegram"
	Token   string `json:"token"`   // Bot token
	ChatID  string `json:"chat_id"` // Chat ID or username
	Enabled bool   `json:"enabled"`
}

// WatcherConfig represents configuration for a URL watcher
type WatcherConfig struct {
	URL          string             `json:"url"`
	Interval     time.Duration      `json:"interval"` // stored as time.Duration but received as seconds
	Status       string             `json:"status"`   // "active" or "disabled"
	Timeout      int                `json:"timeout"`  // timeout in seconds
	Notification NotificationConfig `json:"notification"`
}

// LoadWatcherConfigs loads watcher configurations from the watchers.json file
func LoadWatcherConfigs() ([]WatcherConfig, error) {
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

// SaveWatcherConfigs saves watcher configurations to the watchers.json file
func SaveWatcherConfigs(configs []WatcherConfig) error {
	data, err := json.MarshalIndent(configs, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile("watchers.json", data, 0644)
}

// SendTelegramNotification sends a notification via Telegram bot API
func SendTelegramNotification(token string, chatID string, message string) error {
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
