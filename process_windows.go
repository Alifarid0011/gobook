//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"time"
)

// Windows tree termination is best effort; runtime behavior requires Windows testing.
func configureProcess(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
	}
	cmd.WaitDelay = time.Second
}
func cleanupProcess(cmd *exec.Cmd)                              {}
func openArtifact(root *os.Root, name string) (*os.File, error) { return root.Open(name) }
