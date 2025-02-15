# Starbucks JS Watcher

A Python tool for monitoring and tracking JavaScript changes on starbucks.com.tr website. This tool is designed for bug bounty research and security analysis purposes.

## Features

- Automatically fetches all JavaScript files from starbucks.com.tr
- Combines all JS files into a single file for easy analysis
- Uses Git for version control to track changes over time
- Runs daily checks to monitor updates
- Creates timestamped snapshots
- Maintains a symbolic link to the latest snapshot

## Setup

1. Install requirements:
```bash
pip install -r requirements.txt
```

2. Run the script:
```bash
python main.py
```

## How it Works

- The script runs continuously and checks for updates daily at midnight
- Each snapshot is saved in the `js_snapshots` directory with a timestamp
- Git commits are created automatically for each new snapshot
- Use `git log` and `git diff` to analyze changes between snapshots

## Files

- `latest.js` - Symbolic link to the most recent snapshot
- `js_snapshots/` - Directory containing all JS snapshots
- Each snapshot is named: `starbucks_js_YYYYMMDD_HHMMSS.js`

## Notes

This tool is intended for personal bug bounty research and security analysis. Please respect the website's terms of service and rate limiting policies.
