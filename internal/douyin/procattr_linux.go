//go:build linux

package douyin

import "syscall"

// sysProcAttr makes Chromium die with feedmesh: an orphaned browser would keep
// hundreds of megabytes of memory on a small host.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
