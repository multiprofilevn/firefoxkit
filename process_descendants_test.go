package firefoxkit

import (
	"sort"
	"testing"
)

func sortedKeys(m map[uint32]struct{}) []uint32 {
	out := make([]uint32, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func equalPIDs(got map[uint32]struct{}, want []uint32) bool {
	if len(got) != len(want) {
		return false
	}
	g := sortedKeys(got)
	for i := range want {
		if g[i] != want[i] {
			return false
		}
	}
	return true
}

func TestDescendantPIDs(t *testing.T) {
	// Shape mirrors what a Firefox launch actually looks like: the launched
	// PID (100) re-launches itself as 200, which owns the window and spawns
	// content children (300, 301). 900/901 are an unrelated tree that must
	// never be picked up.
	table := []processEntry{
		{pid: 4, ppid: 0},
		{pid: 100, ppid: 4},
		{pid: 200, ppid: 100},
		{pid: 300, ppid: 200},
		{pid: 301, ppid: 200},
		{pid: 900, ppid: 4},
		{pid: 901, ppid: 900},
	}

	cases := []struct {
		name string
		root uint32
		want []uint32
	}{
		{"direct child and grandchildren", 100, []uint32{100, 200, 300, 301}},
		{"direct child only", 200, []uint32{200, 300, 301}},
		{"leaf has no descendants", 300, []uint32{300}},
		{"unrelated tree is not reachable", 900, []uint32{900, 901}},
		{"pid absent from table yields just itself", 12345, []uint32{12345}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := descendantPIDs(table, tc.root)
			if !equalPIDs(got, tc.want) {
				t.Fatalf("descendantPIDs(%d) = %v, want %v", tc.root, sortedKeys(got), tc.want)
			}
		})
	}
}

// A snapshot can be internally inconsistent (PIDs recycled while the table is
// being read). The walk must terminate on garbage rather than hang the caller.
func TestDescendantPIDsMalformedTableTerminates(t *testing.T) {
	cases := []struct {
		name  string
		table []processEntry
		root  uint32
		want  []uint32
	}{
		{
			name:  "two-node cycle",
			table: []processEntry{{pid: 10, ppid: 20}, {pid: 20, ppid: 10}},
			root:  10,
			want:  []uint32{10, 20},
		},
		{
			name:  "longer cycle",
			table: []processEntry{{pid: 1, ppid: 3}, {pid: 2, ppid: 1}, {pid: 3, ppid: 2}},
			root:  1,
			want:  []uint32{1, 2, 3},
		},
		{
			name:  "self parenting is ignored",
			table: []processEntry{{pid: 7, ppid: 7}, {pid: 8, ppid: 7}},
			root:  7,
			want:  []uint32{7, 8},
		},
		{
			name:  "pid 0 is never a parent",
			table: []processEntry{{pid: 0, ppid: 0}, {pid: 5, ppid: 0}},
			root:  0,
			want:  []uint32{0},
		},
		{
			name:  "empty table",
			table: nil,
			root:  42,
			want:  []uint32{42},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A non-terminating walk shows up as the test binary's own
			// timeout panic, which is the failure signal we want here.
			got := descendantPIDs(tc.table, tc.root)
			if !equalPIDs(got, tc.want) {
				t.Fatalf("descendantPIDs = %v, want %v", sortedKeys(got), tc.want)
			}
		})
	}
}
