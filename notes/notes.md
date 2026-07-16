# Phase 1: MVP

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

Run test server which opens up an http server on port 9090 using:
```bash
python test_server.py
```
