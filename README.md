# Portly

Portly is a small TCP port forwarder. It listens on one address, opens a TCP
connection to a target, and copies traffic in both directions. It currently
supports **TCP only**.

## Build

Requires Go 1.26.5 or newer (as declared in `go.mod`).

```bash
go build -o portly .
```

## Run

With no flags, Portly listens on `127.0.0.1:8080` and forwards to
`127.0.0.1:9001`:

```bash
./portly
```

You can also run it without building a binary first:

```bash
go run .
```

### Flags and examples

| Flag | Default | Purpose |
|---|---|---|
| `-listen` | `127.0.0.1:8080` | Address Portly accepts connections on |
| `-target` | `127.0.0.1:9001` | Address Portly forwards connections to |
| `-idle-timeout` | `5m` | Close a connection after this long with no traffic in either direction; `0` disables |

```bash
# Forward one local port to another
./portly -listen 127.0.0.1:8080 -target 127.0.0.1:9000

# Listen on every IPv4 interface and forward to another host
./portly -listen 0.0.0.0:8080 -target 192.168.1.50:9000

# Close connections after 30 seconds of no traffic, or disable idle timeouts entirely
./portly -idle-timeout 30s
./portly -idle-timeout 0
```

Stop Portly with `Ctrl+C` or `SIGTERM`. It stops accepting new connections and
waits for active connections to finish.

> **Security:** Listening on `0.0.0.0` exposes the port on every IPv4 network
> interface and may make it reachable from your LAN or the internet, depending
> on firewall and router settings. Portly provides no authentication or
> encryption, so use a firewall and bind to `127.0.0.1` unless remote access is
> intentional.

## Platforms

Portly is tested on macOS. It uses Go's standard networking APIs and is expected
to work on Linux and Windows, but those platforms are not currently tested.

## Known limitations

- TCP only; UDP is not supported.
- No authentication, access control, or TLS termination.
- One forwarding rule per process and no connection limit.
- Shutdown waits indefinitely for active connections to close.

Run the tests with `go test ./...`.