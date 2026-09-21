//go:build windows

package process

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
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

type jobObject struct {
	handle windows.Handle
}

func attachJob(cmd *exec.Cmd) (any, error) {
	if cmd.Process == nil {
		return nil, fmt.Errorf("process not started")
	}
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	job := &jobObject{handle: h}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	ph, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_DUP_HANDLE|windows.PROCESS_SUSPEND_RESUME, false, uint32(cmd.Process.Pid))
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	defer windows.CloseHandle(ph)
	if err := windows.AssignProcessToJobObject(h, ph); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return job, nil
}

func closeJob(job any) {
	if j, ok := job.(*jobObject); ok && j != nil && j.handle != 0 {
		_ = windows.CloseHandle(j.handle)
		j.handle = 0
	}
}

func terminateProcessGroup(proc *os.Process, killAfter time.Duration, job any) error {
	if j, ok := job.(*jobObject); ok && j != nil && j.handle != 0 {
		time.Sleep(killAfter)
		_ = windows.TerminateJobObject(j.handle, 1)
		return nil
	}
	if proc == nil {
		return nil
	}
	_ = proc.Kill()
	time.Sleep(killAfter)
	_ = proc.Kill()
	return nil
}
