package main

import (
	"fmt"
	"sync"
	"time"
)

var (
	activeWatchers = make(map[string]*JsWatcher)
	watchersMutex  sync.Mutex
	port           string // Global port variable used across the app
)

// formatInterval converts a duration to a human-readable string
func formatInterval(d time.Duration) string {
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
