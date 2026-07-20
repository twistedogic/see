//go:build windows

package main

import "os/exec"

// Windows CommandContext cancellation kills the shell process directly.
func configureConditionCommand(*exec.Cmd) {}
