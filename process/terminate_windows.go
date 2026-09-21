//go:build windows

package process

import (
	"net"
	"os"
	"os/exec"
	"time"
)

func setProcessGroup(cmd *exec.Cmd) error {
	return nil
}

func prepareControlFD(cmd *exec.Cmd, controlW *os.File) (extra []*os.File, fdNum int, err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, err
	}
	addr := ln.Addr().String()
	cmd.Env = append(cmd.Env, "TIMEOUTX_CONTROL_ADDR="+addr)

	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			_ = controlW.Close()
			return
		}
		defer conn.Close()
		buf := make([]byte, 32*1024)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				_, _ = controlW.Write(buf[:n])
			}
			if err != nil {
				_ = controlW.Close()
				return
			}
		}
	}()
	return nil, -1, nil
}

func terminateProcessGroup(proc *os.Process, killAfter time.Duration) error {
	if proc == nil {
		return nil
	}
	_ = proc.Kill()
	time.Sleep(killAfter)
	_ = proc.Kill()
	return nil
}
