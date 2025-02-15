import os
import requests
from bs4 import BeautifulSoup
import json
from datetime import datetime
import time
import schedule
import re
import subprocess

class StarbucksJSWatcher:
    def __init__(self):
        self.base_url = "https://www.starbucks.com.tr"
        self.js_content = ''
        self.output_dir = 'js_snapshots'
        os.makedirs(self.output_dir, exist_ok=True)

    def fetch_js_urls(self):
        try:
            # Add headers to mimic a browser
            headers = {
                'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36'
            }
            response = requests.get(self.base_url, headers=headers)
            response.raise_for_status()
            
            soup = BeautifulSoup(response.text, 'html.parser')
            
            # Find all script tags
            js_urls = []
            for script in soup.find_all('script', src=True):
                url = script['src']
                if url.startswith('//'):
                    url = 'https:' + url
                elif not url.startswith('http'):
                    url = self.base_url + url if url.startswith('/') else self.base_url + '/' + url
                js_urls.append(url)
            
            return js_urls
        except Exception as e:
            print(f"Error fetching JS URLs: {e}")
            return []

    def download_and_combine_js(self):
        js_urls = self.fetch_js_urls()
        combined_js = []
        
        for url in js_urls:
            try:
                response = requests.get(url)
                response.raise_for_status()
                combined_js.append(f"// Source: {url}")
                combined_js.append(response.text)
                combined_js.append("\n" + "="*80 + "\n")  # Separator between files
            except Exception as e:
                print(f"Error downloading {url}: {e}")
        
        self.js_content = "\n".join(combined_js)

    def save_snapshot(self):
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        filename = f"starbucks_js_{timestamp}.js"
        filepath = os.path.join(self.output_dir, filename)
        
        with open(filepath, 'w', encoding='utf-8') as f:
            f.write(self.js_content)
        
        # Create symbolic link to latest file
        latest_link = os.path.join(self.output_dir, 'latest.js')
        if os.path.exists(latest_link):
            os.remove(latest_link)
        os.symlink(filename, latest_link)
        
        # Git operations
        try:
            # Initialize git repo if not already initialized
            if not os.path.exists('.git'):
                subprocess.run(['git', 'init'])
                
            # Add and commit changes
            subprocess.run(['git', 'add', filepath])
            subprocess.run(['git', 'add', latest_link])
            commit_message = f"Update JS snapshot: {timestamp}"
            subprocess.run(['git', 'commit', '-m', commit_message])
            
            print(f"Saved and committed snapshot: {filepath}")
        except Exception as e:
            print(f"Error in git operations: {e}")

    def check_for_updates(self):
        print(f"Checking for updates at {datetime.now()}")
        self.download_and_combine_js()
        self.save_snapshot()

def main():
    watcher = StarbucksJSWatcher()
    
    # Initial run
    watcher.check_for_updates()
    
    # Schedule daily runs
    schedule.every().day.at("00:00").do(watcher.check_for_updates)
    
    # Keep running
    while True:
        schedule.run_pending()
        time.sleep(60)

if __name__ == "__main__":
    main()
