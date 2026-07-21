# Welcome to Portly

## The Big Picture

Portly is a simple port forwarder that sits between a client and a destination server.

```
Client                  Forwarder                   Destination
curl/browser             your Go program             web server
     │                         │                          │
     │──── connect :8080 ─────▶│                          │
     │                         │──── connect :9000 ──────▶│
     │                         │                          │
     │──── request bytes ─────▶│──── request bytes ──────▶│
     │◀─── response bytes ─────│◀──── response bytes ──────│
```

- Important detail: the forwarder maintains two separate TCP connections:
  1. Client & Forwarder
  2. Forwarder & Destination

## Usage

Build the binary:
```bash
go build -o portly
```

Run it with default settings (listens on `127.0.0.1:8080`, forwards to `127.0.0.1:9001`):
```bash
./portly
```

Or run directly with `go run`:
```bash
go run main.go
```

### Flags

| Flag      | Default            | Description                |
|-----------|---------------------|----------------------------|
| `-listen` | `127.0.0.1:8080`     | Local address to listen on |
| `-target` | `127.0.0.1:9001`     | Remote address to forward to |

### Examples

Forward local port `8080` to a server running on port `9000`:
```bash
./portly -listen 127.0.0.1:8080 -target 127.0.0.1:9000
```

Expose the forwarder on all interfaces and forward to a remote host:
```bash
./portly -listen 0.0.0.0:8080 -target 192.168.1.50:9000
```

Stop the forwarder gracefully with `Ctrl+C` (SIGINT) or `SIGTERM` — it stops accepting new connections and waits for active ones to finish before exiting.

## Testing

Run:
```bash
go test
```
