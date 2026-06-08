//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func configureDaemonProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	const synchronize = 0x00100000
	handle, err := syscall.OpenProcess(synchronize, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	const waitTimeout = 0x00000102
	status, err := syscall.WaitForSingleObject(handle, 0)
	return err == nil && status == waitTimeout
}

func terminateProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
