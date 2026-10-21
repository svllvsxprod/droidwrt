//go:build !linux

package main

import "os/exec"

func configureLeakChild(cmd *exec.Cmd) {}
