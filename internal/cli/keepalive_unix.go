//go:build unix

package cli

import (
	"os/exec"
	"syscall"
)

// detach puts the keepalive into a session of its own, so that it holds nothing
// of the session that started it: neither the pipe a hook's output is read
// through, nor the Ctrl-C meant for the agent.
func detach(started *exec.Cmd) {
	started.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
