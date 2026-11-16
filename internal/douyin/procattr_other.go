//go:build !linux

package douyin

import "syscall"

// sysProcAttr is a no-op off Linux; the service targets Linux hosts.
func sysProcAttr() *syscall.SysProcAttr { return nil }
