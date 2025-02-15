# WatchTover

A tool for monitoring changes in JavaScript files across websites. It tracks changes to JavaScript files by regularly downloading and comparing them against previous versions, storing snapshots in Git repositories for easy diffing and version control.

## Features

- Monitor JavaScript files from any website
- Git-based version control of changes
- Web UI for configuration
- Telegram notifications for changes
- Configurable monitoring intervals
- Automatic retry and timeout mechanisms

## Usage

1. Start the web server:
   ```bash
   go run . -web :9023
   ```

2. Access the web UI at `http://localhost:9023`

3. Add URLs to monitor through the UI

## Configuration

All watcher configurations are stored in `watchers.json`. Each watcher can have:

- URL to monitor
- Check interval
- Status (active/disabled)
- Timeout settings
- Telegram notification settings

## Development

- **Go Version:** 1.21+
- **Dependencies:** See `go.mod`

## Credits

Developed by:
- Navid Mirzaagha ([GitHub](https://github.com/mirzaaghazadeh))
- Website: [navid.tr](https://navid.tr/)

## License

This project is open source. Feel free to use and contribute!

---

**Note:** This tool is intended for bug bounty hunting and security research purposes. Please use responsibly.
