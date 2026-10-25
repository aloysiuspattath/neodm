package engine

import (
	"os"
	"testing"
)

func TestQueueLifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ifd_queue_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	store.Settings.DefaultDownloadDir = tmpDir
	// Prevent auto-starting real HTTP network requests during test
	store.Settings.MaxConcurrentDownloads = 0

	eng := NewEngine(store)

	qm := eng.QueueMgr
	if qm == nil {
		t.Fatal("QueueManager is nil")
	}

	queues := qm.GetAllQueues()
	if len(queues) < 2 {
		t.Fatalf("expected at least 2 default queues, got %d", len(queues))
	}

	// Create custom queue
	customQ, err := qm.CreateQueue("Work Projects", 2)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}
	if customQ.Name != "Work Projects" || customQ.SimultaneousDownloads != 2 {
		t.Fatalf("unexpected custom queue data: %+v", customQ)
	}

	// Add tasks
	t1, err := eng.AddTaskWithQueue("http://example.com/file1.zip", "file1.zip", tmpDir, nil, customQ.ID)
	if err != nil {
		t.Fatalf("failed to add task 1: %v", err)
	}
	t2, err := eng.AddTaskWithQueue("http://example.com/file2.zip", "file2.zip", tmpDir, nil, customQ.ID)
	if err != nil {
		t.Fatalf("failed to add task 2: %v", err)
	}

	qCheck := qm.GetQueue(customQ.ID)
	if len(qCheck.Order) != 2 || qCheck.Order[0] != t1.ID || qCheck.Order[1] != t2.ID {
		t.Fatalf("expected order [t1, t2], got %v", qCheck.Order)
	}

	// Test Reorder
	err = qm.ReorderQueue(customQ.ID, []string{t2.ID, t1.ID})
	if err != nil {
		t.Fatalf("failed to reorder: %v", err)
	}
	qCheck = qm.GetQueue(customQ.ID)
	if qCheck.Order[0] != t2.ID || qCheck.Order[1] != t1.ID {
		t.Fatalf("expected reordered [t2, t1], got %v", qCheck.Order)
	}

	// Move task to main_queue
	err = qm.MoveTask(t2.ID, "main_queue")
	if err != nil {
		t.Fatalf("failed to move task: %v", err)
	}
	qCheck = qm.GetQueue(customQ.ID)
	if len(qCheck.Order) != 1 || qCheck.Order[0] != t1.ID {
		t.Fatalf("expected custom queue to have only t1, got %v", qCheck.Order)
	}

	// Delete custom queue (t1 should be moved to main_queue)
	err = qm.DeleteQueue(customQ.ID)
	if err != nil {
		t.Fatalf("failed to delete queue: %v", err)
	}

	mainQ := qm.GetQueue("main_queue")
	foundT1 := false
	for _, id := range mainQ.Order {
		if id == t1.ID {
			foundT1 = true
			break
		}
	}
	if !foundT1 {
		t.Fatal("expected t1 to be migrated to main_queue upon custom queue deletion")
	}

	// Test Category Detection Pictures
	catPic := DetectCategory("photo.png")
	if catPic != "Pictures" {
		t.Fatalf("expected Pictures, got %s", catPic)
	}
}
