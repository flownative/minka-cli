//go:build !unix

package cli

import "os/exec"

// detach is nothing where there is no session to leave. minka is released for
// unix only; elsewhere it builds, and a keepalive started there stays in the
// session of whoever started it.
func detach(*exec.Cmd) {}
