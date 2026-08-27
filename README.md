# Portly

Portly is a small TCP port forwarder. It listens on one or more addresses,
opens a TCP connection to each rule's target, and copies traffic in both
directions. It currently supports **TCP only**.

## Build

Requires Go 1.26.5 or newer (as declared in `go.mod`).

```bash
go build -o portly ./cmd/portly
```

## Run

Portly reads its forwarding rules from a YAML config file, `portly.yaml` in
the current directory by default:

```bash
./portly
```

You can also run it without building a binary first:

```bash
go run ./cmd/portly
```

### Flags

| Flag | Default | Purpose |
|---|---|---|
| `-config` | `portly.yaml` | Path to the Portly config file |

```bash
./portly -config ./configs/staging.yaml
```

Stop Portly with `Ctrl+C` or `SIGTERM`. It stops accepting new connections on
every rule and waits for active connections to finish.

## Configuration

A config file declares one or more forwarding rules under `rules`:

```yaml
rules:
  - name: web
    listen: 127.0.0.1:8080
    target: 127.0.0.1:3000
    idle_timeout: 5m

  - name: minecraft
    listen: 0.0.0.0:25565
    target: 192.168.1.50:25565
    idle_timeout: 30m
```

| Field | Required | Purpose |
|---|---|---|
| `name` | yes | Identifies the rule in logs; must be unique across the file |
| `listen` | yes | Address Portly accepts connections on for this rule; must be unique across the file |
| `target` | yes | Address Portly forwards connections to; must differ from `listen` |
| `idle_timeout` | no | Close a connection after this long with no traffic in either direction (Go duration syntax, e.g. `30s`, `5m`); defaults to `5m`; `0` disables |

Portly starts every rule as an independent listener and fails to start if the
config has no rules, a duplicate `name`/`listen`, or an invalid field.

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
- No per-rule connection limit.
- Shutdown waits up to 10 seconds per rule for active connections to finish,
  then exits even if some are still active.

Run the tests with `go test ./...`.