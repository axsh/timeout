//go:build unix

package process

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func setProcessGroup(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func prepareControlFD(cmd *exec.Cmd, controlW *os.File) (extra []*os.File, fdNum int, err error) {
	cmd.ExtraFiles = []*os.File{controlW}
	return cmd.ExtraFiles, 3, nil
}

func attachJob(cmd *exec.Cmd) (any, error) { return nil, nil }
func closeJob(job any)                      {}

func terminateProcessGroup(proc *os.Process, killAfter time.Duration, _ any) error {
	if proc == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(proc.Pid)
	if err != nil {
		_ = proc.Signal(syscall.SIGTERM)
		time.Sleep(killAfter)
		_ = proc.Kill()
		return nil
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.Now().Add(killAfter)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pgid, 0); err != nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	return nil
}
