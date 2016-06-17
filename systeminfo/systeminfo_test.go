package systeminfo

import "testing"

func TestInfo(t *testing.T) {
	i, err := New()
	if err != nil {
		t.Fatalf("want err == nil; got %v", err)
	}

	t.Logf("info: %+v\n", i)

	t.Logf("MemoryTotal: %dM\n", i.MemoryTotal/1024/1024)
	t.Logf("MemoryUsage: %dM\n", i.MemoryUsage/1024/1024)
	t.Logf("DiskTotal: %dG\n", i.DiskTotal/1024/1024)
	t.Logf("DiskUsage: %dG\n", i.DiskUsage/1024/1024)

	if i.MemoryTotal == 0 {
		t.Errorf("unexpected memory total %d", i.MemoryTotal)
	}

	if i.MemoryUsage == 0 || i.MemoryUsage > i.MemoryTotal {
		t.Errorf("unexpected memory usage %d", i.MemoryUsage)
	}
}
