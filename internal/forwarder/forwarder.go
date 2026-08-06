package forwarder

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/daanniill/portly/internal/config"
)

type Forwarder struct {
	rule config.RuntimeRule

	listener net.Listener
	connections sync.WaitGroup
}

func New(rule config.RuntimeRule) *Forwarder {
	return &Forwarder{
		rule: rule,
	}
}

func (f *Forwarder) Name() string {
	return f.rule.Name
}

func (f *Forwarder) ListenAddress() string {
	if f.listener == nil {
		return f.rule.Listen
	}

	return f.listener.Addr().String()
}

func (f *Forwarder) TargetAddress() string {
	return f.rule.Target
} 

func (f *Forwarder) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", f.rule.Listen)
	if err != nil {
		return fmt.Errorf("rule %q failed to listen on %s: %w", f.rule.Name, f.rule.Listen, err)
	}

	f.listener = listener

	log.Printf("rule %q started: %s → %s", f.rule.Name, listener.Addr().String(), f.rule.Target)

	go func() {
		<-ctx.Done()

		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("rule %q failed to close listener: %v", f.rule.Name, err)
		}
	}()

	return f.acceptConnections()
}

func (f *Forwarder) acceptConnections() error {
	// Handler listening function
	// will accept traffic at the bound port and run a goroutine as a non-blocking action to handle forwarding the request to the remote location
	for { // we want to continuously listen for requests and not immediately end the function execution
		client, err := f.listener.Accept()
		if err != nil {
			// don't return graceful shutdowns as errors
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("rule %q failed to accept connection: %w", f.rule.Name, err)
		}

		f.connections.Add(1)

		// Handle the actual forwarding to the remote
		go func() {
			defer f.connections.Done()
			handlePortForward(f.rule.Name, client, f.rule.Target, f.rule.IdleTimeout)
		}()
	}
}

func (f *Forwarder) Wait() {
	f.connections.Wait()

	log.Printf("rule %q has no remaining active connections", f.rule.Name)
}