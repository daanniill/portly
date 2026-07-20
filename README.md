# Welcome to Portly

## The Big Picture

Portly is a simple port forwarder that sits between a client and a destination server

Client                  Forwarder                   Destination
curl/browser             your Go program             web server
     │                         │                          │
     │──── connect :8080 ─────▶│                          │
     │                         │──── connect :9000 ──────▶│
     │                         │                          │
     │──── request bytes ─────▶│──── request bytes ─────-▶│
     │◀─── response bytes ─────│◀── response bytes ──────-|

- Important detail: the forwarder maintains two seperate TCP connections
  1. Client & Forwarder
  2. Forwarder & Destination

## Testing

Run tests by running:
```bash
go test
```
