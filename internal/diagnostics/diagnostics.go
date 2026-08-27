package diagnostics

import (
	"fmt"
	"net"
	"time"
)

type DiagnosticResult struct {
	Check string
	Success bool
	Message string
}

// checks if target is available for portly to open
func checkTarget(remoteAddress string) DiagnosticResult {
	target, err := net.DialTimeout("tcp", remoteAddress, 5*time.Second)

	if err != nil {
		return DiagnosticResult{
			Check: "target",
			Success: false,
			Message: fmt.Sprintf("cannot reach %s: %v", remoteAddress, err),
		}
	}

	defer target.Close()

	return DiagnosticResult{
			Check: "target",
			Success: true,
			Message: fmt.Sprintf("%s is reachable", target),
		}
}

// checks if listener 
func checkListenAddress(address string) DiagnosticResult {
	listener, err := net.Listen("tcp", address)

	if err != nil {
		return DiagnosticResult{
			Check: "target",
			Success: false,
			Message: fmt.Sprintf("cannot listen on %s: %v", address, err),
		}
	}

	defer listener.Close()

	return DiagnosticResult{
			Check: "target",
			Success: true,
			Message: fmt.Sprintf("%s is available", listener.Addr()),
	}
}