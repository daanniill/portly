# Prototype 1: one-port forwarder

## Architecture
Client                  Forwarder                   Destination
curl/browser             your Go program             web server
     │                         │                          │
     │──── connect :8080 ─────▶│                          │
     │                         │──── connect :9000 ──────▶│
     │                         │                          │
     │──── request bytes ─────▶│──── request bytes ─────-▶│
     │◀─── response bytes ─────│◀── response bytes ──────-│

## Opening a local enterance

```go
listener, err := net.Listen("tcp", ":8080")
```
- This tells the os: Allow my program to receive TCP connections addressed to local port 8080.
- It returns a net.Listener. A listener is not a connection to one particular client. Think of it as a reception desk waiting for visitors.

## Waiting for a Client
```go
client, err := listener.Accept()
```
- Accept() waits until somebody connects. Once a client arrives, it returns a new net.Conn representing that individual connection. The listener stays open so it can accept additional clients

listener
├── waits on port 8080
├── client connection 1
├── client connection 2
└── client connection 3

## Making the outgoing connection
```go
target, err := net.Dial("tcp", "127.0.0.1:9000")
```
- Once a client connects, your forwarder needs to connect to the real destination
- Dial is the client-side counterpart to Listen
  - Listen → wait for someone to connect to you
  - Dial   → connect to someone else
- Go defines Dial as connecting to an address on the selected network. TCP destinations use the host:port form

Both client and target objects are `net.Conn` which provides methods including:
```go
Read()
Write()
Close()
LocalAddr()
RemoteAddr()
```
A net.Conn can both read and write data, matching TCP’s bidirectional behavior.

## Move bytes between connections
```go
io.Copy(destination, source)
```
It repeatedly:
1. Reads bytes from src.
2. Writes those bytes to dst.
3. Continues until the source closes or an error occurs

In our program, we need to both read from client and write to target, as well as read from target and write to client
Visually:
client ─────────▶ target
client ◀───────── target

Both copy statements must run concurrently:
```go
go io.Copy(target, client)
go io.Copy(client, target)
```
TCP permits data to flow in both directions independently; closing the sending direction does not inherently prevent continuing to receive.

## What the chaannel is doing
```go
done := make(chan error, 2)
```
Each copying goroutine sends its result into the channel
```go
done <- err
```
The handler waits here:
```go
copyErr := <- done
```
The flow is:
Goroutine 1: client → target ──┐
                               ├── done channel
Goroutine 2: target → client ──┘
                                      │
Handler waits:                 copyErr := <-done