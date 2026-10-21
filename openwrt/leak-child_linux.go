package main

import (
	"os/exec"
	"syscall"
)

func configureLeakChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}
