import http.server
import socketserver
import argparse

# parser init
parser = argparse.ArgumentParser(description="A script that opens up a simple http test server using TCP")

# args
parser.add_argument("-p", "--port", type=int, default=9001, help="Port on which server opens (default: 9001)")

# parse the data
args = parser.parse_args()

PORT = args.port

Handler = http.server.SimpleHTTPRequestHandler

with socketserver.TCPServer(("", PORT), Handler) as httpd:
    print(f"Serving at port {PORT}")
    # Start the server and keep it running until you stop the script
    httpd.serve_forever()