#!/usr/bin/env python3
"""
Simple HTTP server for testing the ADS-B Scope PWA locally.
Serves static files from the web/static directory.
"""

import http.server
import socketserver
import os
import sys

PORT = 8000
DIRECTORY = "static"

class MyHTTPRequestHandler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=DIRECTORY, **kwargs)
    
    def end_headers(self):
        # Add CORS headers for local development
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'GET, POST, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Content-Type')
        
        # Cache control for development
        self.send_header('Cache-Control', 'no-cache, no-store, must-revalidate')
        
        super().end_headers()

def main():
    # Change to the script directory
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    
    with socketserver.TCPServer(("", PORT), MyHTTPRequestHandler) as httpd:
        print(f"🚀 ADS-B Scope PWA Server")
        print(f"📡 Serving at http://localhost:{PORT}")
        print(f"📁 Directory: {os.path.join(os.getcwd(), DIRECTORY)}")
        print(f"\n💡 Open http://localhost:{PORT} in your browser")
        print(f"   Demo login: admin / admin\n")
        print("Press Ctrl+C to stop\n")
        
        try:
            httpd.serve_forever()
        except KeyboardInterrupt:
            print("\n\n👋 Server stopped")
            sys.exit(0)

if __name__ == "__main__":
    main()
