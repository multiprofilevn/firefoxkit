//go:build windows

package firefoxkit

import (
	"fmt"
	"syscall"
	"unsafe"
)

// th32csSnapProcess is TH32CS_SNAPPROCESS — include the process list in the
// toolhelp snapshot.
const th32csSnapProcess = 0x00000002

// snapshotProcessTable reads the whole process table via the toolhelp
// snapshot API. It is a package var so tests can substitute a fixture; the
// real implementation is Win32-only and cannot run under `go test` on a
// build agent without live processes.
var snapshotProcessTable = win32ProcessTable

func win32ProcessTable() ([]processEntry, error) {
	h, err := syscall.CreateToolhelp32Snapshot(th32csSnapProcess, 0)
	if err != nil {
		return nil, fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}
	defer syscall.CloseHandle(h)

	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	if err := syscall.Process32First(h, &e); err != nil {
		return nil, fmt.Errorf("Process32First: %w", err)
	}
	// A snapshot is a fixed-size copy taken at CreateToolhelp32Snapshot time,
	// so this loop is bounded by however many processes existed at that
	// instant — it cannot be extended by processes spawning underneath us.
	out := make([]processEntry, 0, 256)
	for {
		out = append(out, processEntry{pid: e.ProcessID, ppid: e.ParentProcessID})
		if err := syscall.Process32Next(h, &e); err != nil {
			if err == syscall.ERROR_NO_MORE_FILES {
				break
			}
			return nil, fmt.Errorf("Process32Next: %w", err)
		}
	}
	return out, nil
}
