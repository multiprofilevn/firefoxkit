package firefoxkit

// processEntry is one row of the OS process table, reduced to the only two
// fields the window lookup needs. Keeping it platform-neutral puts the
// descendant walk behind a seam: the Win32 snapshot lives in
// process_descendants_windows.go, the logic below is testable everywhere.
type processEntry struct {
	pid  uint32
	ppid uint32
}

// descendantPIDs returns root plus every process transitively spawned by it.
//
// WHY THIS EXISTS: Firefox uses a launcher process. The PID returned by
// Launch re-launches itself and hands off, so the top-level
// MozillaWindowClass window is owned by a DESCENDANT, not the PID we hold.
// Measured on FirefoxCore_v149 (GPM build): launched PID 117448, the only
// visible MozillaWindowClass window belonged to PID 164404. Matching windows
// by exact PID equality — what findFirefoxHWND used to do — never matched, so
// Browser.HWND() always failed and Focus/ContentOffset/WindowRect/WindowScale
// and the foreground guard all silently degraded to their fallbacks.
//
// The walk is bounded by the snapshot: every PID is visited at most once, so a
// malformed table (a cycle, or a process listed as its own parent) terminates
// instead of looping. PID 0 is never treated as a parent — on Windows it is
// the idle process and orphaned entries commonly point at it.
func descendantPIDs(table []processEntry, root uint32) map[uint32]struct{} {
	out := map[uint32]struct{}{root: {}}
	if len(table) == 0 {
		return out
	}

	children := make(map[uint32][]uint32, len(table))
	for _, e := range table {
		if e.ppid == 0 || e.pid == 0 || e.pid == e.ppid {
			continue // orphan, idle process, or self-parenting garbage
		}
		children[e.ppid] = append(children[e.ppid], e.pid)
	}

	queue := []uint32{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, kid := range children[cur] {
			if _, seen := out[kid]; seen {
				continue // cycle guard: also the loop bound (≤ len(table) visits)
			}
			out[kid] = struct{}{}
			queue = append(queue, kid)
		}
	}
	return out
}
