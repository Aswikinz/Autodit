//go:build linux

package analysis

import "syscall"

func limitWorker() error {
	// Data includes native allocations and anonymous mmap on Linux, while allowing
	// Go's large reserved virtual address range. CPU is independent of wall time.
	if err := syscall.Setrlimit(syscall.RLIMIT_DATA, &syscall.Rlimit{Cur: 512 * 1024 * 1024, Max: 512 * 1024 * 1024}); err != nil {
		return err
	}
	return syscall.Setrlimit(syscall.RLIMIT_CPU, &syscall.Rlimit{Cur: 15, Max: 15})
}
