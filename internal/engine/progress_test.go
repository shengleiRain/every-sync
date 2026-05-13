package engine

import "testing"

func TestProgressTrackerLifecycle(t *testing.T) {
	tracker := NewProgressTracker()

	tracker.StartSync(7, "photos", "up", 3)
	tracker.StartTask(7, "upload", "/camera/IMG_1042.CR3", 1024)
	tracker.ChunkTransferred(7, "/camera/IMG_1042.CR3", 256, 1024)

	snapshots := tracker.Snapshots()
	if len(snapshots) != 1 {
		t.Fatalf("snapshots len = %d, want 1", len(snapshots))
	}
	got := snapshots[0]
	if got.PairID != 7 || got.Status != "syncing" || got.Direction != "up" {
		t.Fatalf("snapshot header = %+v", got)
	}
	if got.ActiveFile == nil {
		t.Fatalf("ActiveFile is nil")
	}
	if got.ActiveFile.Path != "/camera/IMG_1042.CR3" {
		t.Fatalf("path = %q", got.ActiveFile.Path)
	}
	if got.ActiveFile.BytesTransferred != 256 || got.ActiveFile.BytesTotal != 1024 {
		t.Fatalf("bytes = %d/%d", got.ActiveFile.BytesTransferred, got.ActiveFile.BytesTotal)
	}
	if got.ActiveFile.Percent != 25 {
		t.Fatalf("percent = %.1f, want 25", got.ActiveFile.Percent)
	}

	tracker.CompleteTask(7, "upload", "/camera/IMG_1042.CR3")
	gotPtr := tracker.Snapshot(7)
	if gotPtr == nil {
		t.Fatalf("snapshot missing while sync is active")
	}
	if gotPtr.ActiveFile != nil {
		t.Fatalf("ActiveFile after completion = %+v, want nil", gotPtr.ActiveFile)
	}
	if gotPtr.FilesSynced != 1 {
		t.Fatalf("FilesSynced = %d, want 1", gotPtr.FilesSynced)
	}

	tracker.FinishSync(7)
	if got := tracker.Snapshot(7); got != nil {
		t.Fatalf("snapshot after FinishSync = %+v, want nil", got)
	}
}

func TestProgressTrackerFailedTask(t *testing.T) {
	tracker := NewProgressTracker()
	tracker.StartSync(8, "docs", "down", 1)
	tracker.StartTask(8, "download", "/docs/report.pdf", 2048)
	tracker.FailTask(8, "download", "/docs/report.pdf", "network timeout")

	got := tracker.Snapshot(8)
	if got == nil {
		t.Fatalf("snapshot missing after failed task")
	}
	if got.Status != "failed" {
		t.Fatalf("Status = %q, want failed", got.Status)
	}
	if got.Error != "network timeout" {
		t.Fatalf("Error = %q", got.Error)
	}
	if got.ActiveFile != nil {
		t.Fatalf("ActiveFile = %+v, want nil", got.ActiveFile)
	}
}
