//go:build windows

package firefoxkit

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
)

const (
	// JobObjectExtendedLimitInformation class + the flag that terminates every
	// process in the job once the last handle to it closes.
	jobObjectExtendedLimitInfoClass = 9
	jobObjectLimitKillOnJobClose    = 0x00002000

	// OpenProcess rights: quota+terminate to assign into a job, limited query
	// to read the exit code.
	processTerminate               = 0x0001
	processSetQuota                = 0x0100
	processQueryLimitedInformation = 0x1000

	// GetExitCodeProcess reports this while the process is still running.
	stillActive = 259
)

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// processTreeReaper kills a spawned process together with every descendant it
// created. Killing the parent alone is not enough for Firefox: its content/GPU
// children survive, keep the profile dir locked and hold the parent's pipes
// open, which parks the goroutines reading them. A Job Object created at spawn
// time with KILL_ON_JOB_CLOSE takes the whole tree down when the handle closes;
// when the process cannot be assigned to a job (it already sits in one that
// forbids nesting), the reaper falls back to `taskkill /T /F`.
//
// Callers are serialized by construction (Launch failure path, or Close under
// closeOnce), so the handle needs no lock of its own.
type processTreeReaper struct {
	pid int
	job syscall.Handle // 0 → taskkill fallback
	why string         // why the job is unavailable, for diagnostics
}

// newProcessTreeReaper builds the reaper for an already-started process. It
// never fails: a process that cannot be put in a job still gets the taskkill
// fallback at reap time.
func newProcessTreeReaper(pid int) *processTreeReaper {
	r := &processTreeReaper{pid: pid}
	job, err := createKillOnCloseJob()
	if err != nil {
		r.why = err.Error()
		return r
	}
	if err := assignProcessToJob(job, pid); err != nil {
		_ = syscall.CloseHandle(job)
		r.why = err.Error()
		return r
	}
	r.job = job
	return r
}

// reap kills the process tree. Idempotent: once the job handle is closed (or
// the process is already gone) further calls are cheap no-ops.
func (r *processTreeReaper) reap() error {
	if r == nil {
		return nil
	}
	if r.job != 0 {
		job := r.job
		r.job = 0
		if err := syscall.CloseHandle(job); err != nil {
			return fmt.Errorf("close job for pid=%d: %w", r.pid, err)
		}
		return nil
	}
	if !processAlive(r.pid) {
		return nil
	}
	out, err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(r.pid)).CombinedOutput()
	// taskkill also errors when the tree died between the check and the call —
	// only a still-running process counts as a failure.
	if err != nil && processAlive(r.pid) {
		return fmt.Errorf("taskkill /T /F /PID %d: %v: %s", r.pid, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// mode names the mechanism that will do the killing. Diagnostics only.
func (r *processTreeReaper) mode() string {
	switch {
	case r == nil:
		return "none"
	case r.job != 0:
		return "job object"
	default:
		return "taskkill fallback (" + r.why + ")"
	}
}

func createKillOnCloseJob() (syscall.Handle, error) {
	h, _, e := procCreateJobObjectW.Call(0, 0)
	if h == 0 {
		return 0, fmt.Errorf("CreateJobObject: %v", e)
	}
	job := syscall.Handle(h)
	var info jobObjectExtendedLimitInformation
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	ret, _, e := procSetInformationJobObject.Call(
		uintptr(job),
		jobObjectExtendedLimitInfoClass,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if ret == 0 {
		_ = syscall.CloseHandle(job)
		return 0, fmt.Errorf("SetInformationJobObject: %v", e)
	}
	return job, nil
}

func assignProcessToJob(job syscall.Handle, pid int) error {
	h, err := syscall.OpenProcess(processSetQuota|processTerminate, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("OpenProcess %d: %w", pid, err)
	}
	defer syscall.CloseHandle(h)
	if ret, _, e := procAssignProcessToJobObject.Call(uintptr(job), uintptr(h)); ret == 0 {
		return fmt.Errorf("AssignProcessToJobObject %d: %v", pid, e)
	}
	return nil
}

// processAlive reports whether pid exists and has not exited yet. A pid we no
// longer hold a handle to may have been recycled; callers use this only to skip
// work on an already dead tree, where that is harmless.
func processAlive(pid int) bool {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
