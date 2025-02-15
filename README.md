# WatchTover GitDif

A Go tool that watches JavaScript files on web applications and tracks changes using Git. Perfect for bug bounty hunting and monitoring dynamic JS files in React/Vue applications where filenames change between builds.

## Features

- 🔍 Monitors any web URL for JavaScript files
- 📦 Combines all JS files into one for easy diffing
- 🕒 Configurable check intervals
- 🔄 Creates separate Git repositories for each monitored URL
- 💾 Tracks changes over time using Git commits
- 🔗 Handles both relative and absolute JS URLs
- 🏗️ Works with dynamic filenames (React/Vue builds)

## Installation

```bash
go install github.com/watchtover-gitdif@latest
```

## Usage

Basic usage (monitors http://localhost:8000 every 10 seconds):
```bash
watchtover-gitdif
```

Monitor a specific URL with custom interval:
```bash
watchtover-gitdif -url https://example.com -interval 30
```

### Command Line Options

- `-url`: URL to monitor (default: http://localhost:8000)
- `-interval`: Check interval in seconds (default: 10)

## How It Works

1. The tool creates a separate Git repository for each monitored URL in the `js_snapshots/<sanitized-url>` directory
2. Every interval:
   - Fetches the webpage
   - Extracts all JavaScript files
   - Combines them into a single file with metadata
   - Creates a Git commit if changes are detected

## Viewing Changes

Navigate to the URL's Git repository:
```bash
cd js_snapshots/<sanitized-url>
```

View commit history:
```bash
git log
```

See changes between commits:
```bash
git diff HEAD~1 HEAD
```

## Example

Monitor a React application and check every 30 seconds:
```bash
watchtover-gitdif -url http://localhost:3000 -interval 30
```

The tool will:
1. Create a repository at `js_snapshots/localhost_3000/`
2. Save all JS files as `combined.js`
3. Create Git commits when changes are detected
4. Show real-time logging of its activities

## Bug Bounty Hunting Use Cases

- Monitor for exposed sensitive information in JS files
- Track changes in API endpoints
- Discover new features before they're officially released
- Find forgotten debug code in production builds
- Monitor third-party script changes

## Development

Requirements:
- Go 1.21 or higher

Building from source:
```bash
git clone https://github.com/watchtover-gitdif
cd watchtover-gitdif
go build
```

## License

MIT License
