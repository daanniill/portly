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