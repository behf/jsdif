# JSDif Watcher 🕵️‍♂️ V.1.0.2

![Alt Text](showcase.gif)

A powerful JavaScript monitoring tool for bug bounty hunters. Track changes in JavaScript files across websites, detect new attack surfaces, and stay ahead of security vulnerabilities.

## 🎯 Features

- 🔍 Monitor JavaScript files from any website
- 📊 Git-based version control of changes
- 🌐 Clean web UI for easy configuration
- 🔔 Telegram notifications for instant alerts
- ⚡ Configurable monitoring intervals
- 🔄 Automatic retry and timeout mechanisms
- 📝 Efficient diff viewing for quick analysis
- 🛡️ Automatic error handling and retry mechanisms
- 🎮 Web interface for easy management of monitored URLs

## 🚀 Quick Start

### Docker Installation (Recommended)

The easiest way to run JSDif is using Docker:

```bash
# Clone the repository
git clone https://github.com/behf/jsdif.git
cd jsdif

# Start with Docker Compose
docker compose up -d
```

Access the web interface at `http://localhost:9093`

For detailed Docker setup instructions including:
- Authentication configuration
- Custom port configuration
- Data persistence management
- Container management
- Troubleshooting guide

👉 See [DOCKER.md](DOCKER.md)

### Manual Installation

#### Requirements

- Go 1.20 or higher
- Git installed and accessible from PATH
- Linux, macOS, or Windows operating system

#### Installation

```bash
go install github.com/behf/jsdif@latest
```

### Usage

Basic usage without authentication:
```bash
jsdif run -p 9093
```

Optional basic authentication:
```bash
jsdif -u admin --password secret -p 9093 run
```

Available flags:
- `-p`: Port number (default: 9093)
- `-u`: (Optional) Username for authentication on web gui
- `--password`: (Optional) Password for authentication on web gui

Access the web interface at `http://localhost:9093` to start monitoring your targets.

## 💻 Web Interface Features

- Add/Edit/Remove monitored URLs
- Configure monitoring intervals per URL
- View real-time status of each watcher
- Set custom timeout values
- Browse through historical changes
- View detailed diffs between versions
- Toggle monitoring status (active/disabled)

## 🔔 Notification Setup

### Telegram Notifications

1. Create a new bot using [@BotFather](https://t.me/botfather) on Telegram
2. Get your bot token
3. Get your chat ID (you can use [@userinfobot](https://t.me/userinfobot))
4. Configure notifications in the web interface:
   - Enable notifications
   - Select Telegram as the notification type
   - Enter your bot token
   - Enter your chat ID

## 🔥 Bug Bounty Use Cases

- 🎯 Track new JavaScript endpoints and APIs
- 🔑 Monitor for leaked sensitive information
- 🛡️ Detect changes in security controls
- 🚀 Find new features before they're officially released
- ⚠️ Identify removed security checks
- 📦 Track third-party script changes
- 🔒 Monitor authentication/authorization changes

## 🔨 Building from Source

```bash
git clone https://github.com/behf/jsdif.git
cd jsdif
go build -o jsdif
```

## ⚙️ Configuration

### Command Line Options

- **Port**: `-p` flag to set the server port (default: 9093)
- **Authentication**: Optional basic auth protection
  - `-u`: Username for web interface access
  - `--password`: Password for web interface access

### Web Interface Settings

The web interface allows you to configure:

- **URL**: The target website to monitor
- **Interval**: How often to check for changes (in minutes)
- **Status**: Active or Disabled
- **Timeout**: Maximum number of retry attempts before disabling
- **Notifications**: Telegram notification settings
  - Enable/Disable notifications
  - Bot Token
  - Chat ID

## 🐛 Reporting Issues

If you encounter any bugs or have feature requests, please:

1. Check the existing issues on GitHub
2. Create a new issue with:
   - Detailed description of the problem
   - Steps to reproduce
   - Expected vs actual behavior
   - System information (OS, Go version)

## 📝 License

This project is open source. Feel free to use and contribute!

---

**⚠️ Note:** This tool is intended for bug bounty hunting and security research. Use responsibly and follow program policies.
