package terminal

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func processCleanup(p *os.Process) func() {
	job, err := windows.CreateJobObject(nil, nil)
	if err == nil {
		limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
		if err == nil {
			var process windows.Handle
			process, err = windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
			if err == nil {
				err = windows.AssignProcessToJobObject(job, process)
				_ = windows.CloseHandle(process)
			}
		}
		if err == nil {
			return func() { _ = windows.CloseHandle(job) }
		}
		_ = windows.CloseHandle(job)
	}
	return func() { _ = p.Kill() }
}
