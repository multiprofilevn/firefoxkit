//go:build windows

package firefoxkit

import (
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// startPingTree spawns a cmd.exe that runs a short ping and then a long one.
// The long child only appears ~2s in, so a test can create the reaper first
// and still exercise a child that was spawned AFTER the job object existed —
// which is what the production spawn path looks like.
func startPingTree(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("cmd.exe", "/c", "ping -n 3 127.0.0.1 >NUL & ping -n 60 127.0.0.1 >NUL")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start ping tree: %v", err)
	}
	return cmd
}

// childPIDs lists the direct children of ppid via a process snapshot.
func childPIDs(t *testing.T, ppid int) []int {
	t.Helper()
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatalf("CreateToolhelp32Snapshot: %v", err)
	}
	defer syscall.CloseHandle(snap)
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []int
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		if int(e.ParentProcessID) == ppid {
			out = append(out, int(e.ProcessID))
		}
	}
	return out
}

// waitForLateChild returns a child of ppid that was spawned after the tree's
// first ping finished (~2s), i.e. after the caller built its reaper.
func waitForLateChild(t *testing.T, ppid int, spawnedAt time.Time) int {
	t.Helper()
	if d := 3*time.Second - time.Since(spawnedAt); d > 0 {
		time.Sleep(d)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if kids := childPIDs(t, ppid); len(kids) > 0 {
			return kids[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("no child process under pid=%d", ppid)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// assertDeadWithin polls until every pid is gone or the budget expires.
func assertDeadWithin(t *testing.T, budget time.Duration, pids ...int) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for {
		alive := []int{}
		for _, pid := range pids {
			if processAlive(pid) {
				alive = append(alive, pid)
			}
		}
		if len(alive) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pids still alive after %v: %v", budget, alive)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func killTree(cmd *exec.Cmd) {
	_ = (&processTreeReaper{pid: cmd.Process.Pid}).reap()
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

// A job object created at spawn must take the whole tree down when its handle
// closes — parent AND the child spawned after assignment.
func TestProcessTreeReaper_JobObjectKillsChildren(t *testing.T) {
	cmd := startPingTree(t)
	spawnedAt := time.Now()
	r := newProcessTreeReaper(cmd.Process.Pid)
	t.Cleanup(func() { killTree(cmd) })
	if r.job == 0 {
		t.Skipf("job object unavailable: %s", r.mode())
	}
	child := waitForLateChild(t, cmd.Process.Pid, spawnedAt)
	if err := r.reap(); err != nil {
		t.Fatalf("reap: %v", err)
	}
	assertDeadWithin(t, 2*time.Second, cmd.Process.Pid, child)
}

// When the process could not be assigned to a job, the taskkill fallback must
// still take the whole tree down.
func TestProcessTreeReaper_TaskkillFallbackKillsChildren(t *testing.T) {
	cmd := startPingTree(t)
	spawnedAt := time.Now()
	r := &processTreeReaper{pid: cmd.Process.Pid} // job == 0 → fallback path
	t.Cleanup(func() { killTree(cmd) })
	child := waitForLateChild(t, cmd.Process.Pid, spawnedAt)
	if err := r.reap(); err != nil {
		t.Fatalf("reap: %v", err)
	}
	assertDeadWithin(t, 2*time.Second, cmd.Process.Pid, child)
}

// Reaping a process that already exited is a cheap no-op on both paths.
func TestProcessTreeReaper_AlreadyDeadReturnsNil(t *testing.T) {
	for _, tc := range []struct {
		name string
		job  bool
	}{{"job", true}, {"taskkill", false}} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("cmd.exe", "/c", "exit 0")
			if err := cmd.Start(); err != nil {
				t.Fatalf("start: %v", err)
			}
			var r *processTreeReaper
			if tc.job {
				r = newProcessTreeReaper(cmd.Process.Pid)
			} else {
				r = &processTreeReaper{pid: cmd.Process.Pid}
			}
			_ = cmd.Wait()
			start := time.Now()
			if err := r.reap(); err != nil {
				t.Fatalf("reap on dead process: %v", err)
			}
			if elapsed := time.Since(start); elapsed > 2*time.Second {
				t.Fatalf("reap on dead process took %v, want prompt return", elapsed)
			}
		})
	}
}

// reapProcess is the Close-side contract: kill the tree, then trust only the
// Wait signal. Happy path — the process really dies, no error.
func TestReapProcess_KillsTreeAndReturnsNil(t *testing.T) {
	cmd := startPingTree(t)
	spawnedAt := time.Now()
	done := make(chan struct{})
	b := &Browser{
		proc:      cmd.Process,
		closeProc: true,
		procTree:  newProcessTreeReaper(cmd.Process.Pid),
		procDone:  done,
	}
	go func() { _ = cmd.Wait(); close(done) }()
	child := waitForLateChild(t, cmd.Process.Pid, spawnedAt)
	if err := b.reapProcess(); err != nil {
		t.Fatalf("reapProcess: %v", err)
	}
	assertDeadWithin(t, 2*time.Second, cmd.Process.Pid, child)
}

// A kill that "succeeds" while the process keeps running must surface as a
// real error — the Wait signal never arriving is the only evidence that counts.
func TestReapProcess_ReportsSurvivorWhenWaitNeverSignals(t *testing.T) {
	cmd := startPingTree(t)
	t.Cleanup(func() { killTree(cmd) })
	b := &Browser{
		proc:      cmd.Process,
		closeProc: true,
		procTree:  newProcessTreeReaper(cmd.Process.Pid),
		procDone:  make(chan struct{}), // never closed: no Wait owner in this test
	}
	restore := procExitTimeout
	procExitTimeout = 200 * time.Millisecond
	defer func() { procExitTimeout = restore }()

	err := b.reapProcess()
	if err == nil {
		t.Fatal("reapProcess returned nil while the process exit was never observed")
	}
	if !strings.Contains(err.Error(), "survived kill") {
		t.Fatalf("error = %v, want a survived-kill report", err)
	}
}
