package engine

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// QueueRecord defines a named download queue with custom scheduling and concurrency rules.
type QueueRecord struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	SimultaneousDownloads int       `json:"simultaneousDownloads"` // 1 = strictly sequential, 2+ = parallel
	Order                 []string  `json:"order"`                 // Ordered list of Task IDs
	Running               bool      `json:"running"`
	ScheduleEnabled       bool      `json:"scheduleEnabled"`
	StartTime             string    `json:"startTime"` // "HH:MM" 24h format, e.g. "02:00"
	StopTime              string    `json:"stopTime"`  // "HH:MM" 24h format, e.g. "06:00"
	DaysOfWeek            []int     `json:"daysOfWeek"`
	OnComplete            string    `json:"onComplete"` // "none", "close_ifd", "sleep", "shutdown"
	CreatedAt             time.Time `json:"createdAt"`
}

// QueueManager coordinates queue lifecycle, task progression, and automated schedules.
type QueueManager struct {
	mu     sync.RWMutex
	queues map[string]*QueueRecord
	engine *Engine
	store  *Store
	stopCh chan struct{}
}

// NewQueueManager initializes queue management with default Main and Night queues.
func NewQueueManager(engine *Engine, store *Store) *QueueManager {
	qm := &QueueManager{
		queues: make(map[string]*QueueRecord),
		engine: engine,
		store:  store,
		stopCh: make(chan struct{}),
	}

	// Restore queues from store or initialize defaults
	if store.Settings.Queues != nil && len(store.Settings.Queues) > 0 {
		for id, q := range store.Settings.Queues {
			qm.queues[id] = q
		}
		if q, exists := qm.queues["main_queue"]; exists {
			q.Running = true
		}
	} else {
		// Default Main Download Queue
		qm.queues["main_queue"] = &QueueRecord{
			ID:                    "main_queue",
			Name:                  "Main Download Queue",
			SimultaneousDownloads: 4,
			Order:                 []string{},
			Running:               true,
			ScheduleEnabled:       false,
			OnComplete:            "none",
			CreatedAt:             time.Now(),
		}
		// Default Night Queue (sequential, scheduled 02:00 to 06:00, auto-sleep)
		qm.queues["night_queue"] = &QueueRecord{
			ID:                    "night_queue",
			Name:                  "Night Download Queue",
			SimultaneousDownloads: 1,
			Order:                 []string{},
			Running:               false,
			ScheduleEnabled:       true,
			StartTime:             "02:00",
			StopTime:              "06:00",
			OnComplete:            "sleep",
			CreatedAt:             time.Now(),
		}
		qm.persist()
	}

	go qm.scheduleLoop()
	return qm
}

func (qm *QueueManager) persist() {
	qm.store.Settings.Queues = qm.queues
	_ = qm.store.SaveSettings()
}

// GetAllQueues returns a snapshot of all configured queues.
func (qm *QueueManager) GetAllQueues() []*QueueRecord {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	result := make([]*QueueRecord, 0, len(qm.queues))
	for _, q := range qm.queues {
		cp := *q
		result = append(result, &cp)
	}
	return result
}

// GetQueue returns a queue by ID.
func (qm *QueueManager) GetQueue(id string) *QueueRecord {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	return qm.queues[id]
}

// CreateQueue registers a new custom queue.
func (qm *QueueManager) CreateQueue(name string, simultaneous int) (*QueueRecord, error) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if simultaneous < 1 {
		simultaneous = 1
	}

	id := fmt.Sprintf("queue_%d", time.Now().UnixNano())
	q := &QueueRecord{
		ID:                    id,
		Name:                  name,
		SimultaneousDownloads: simultaneous,
		Order:                 []string{},
		Running:               false,
		ScheduleEnabled:       false,
		OnComplete:            "none",
		CreatedAt:             time.Now(),
	}

	qm.queues[id] = q
	qm.persist()
	return q, nil
}

// DeleteQueue removes a custom queue and moves its remaining tasks to main_queue.
func (qm *QueueManager) DeleteQueue(id string) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if id == "main_queue" {
		return fmt.Errorf("cannot delete default main download queue")
	}

	q, exists := qm.queues[id]
	if !exists {
		return fmt.Errorf("queue not found")
	}

	mainQ := qm.queues["main_queue"]
	if mainQ != nil {
		for _, taskID := range q.Order {
			mainQ.Order = append(mainQ.Order, taskID)
			t := qm.engine.GetTask(taskID)
			if t != nil {
				t.QueueID = "main_queue"
				qm.engine.updateTask(t)
			}
		}
	}

	delete(qm.queues, id)
	qm.persist()
	return nil
}

// StartQueue begins active download processing for the specified queue.
func (qm *QueueManager) StartQueue(id string) error {
	qm.mu.Lock()
	q, exists := qm.queues[id]
	if !exists {
		qm.mu.Unlock()
		return fmt.Errorf("queue not found")
	}
	q.Running = true
	qm.persist()
	qm.mu.Unlock()

	qm.engine.checkQueue()
	return nil
}

// StopQueue pauses active processing for the specified queue.
func (qm *QueueManager) StopQueue(id string) error {
	qm.mu.Lock()
	q, exists := qm.queues[id]
	if !exists {
		qm.mu.Unlock()
		return fmt.Errorf("queue not found")
	}
	q.Running = false
	qm.persist()
	qm.mu.Unlock()

	// Pause downloading tasks belonging to this queue
	for _, taskID := range q.Order {
		t := qm.engine.GetTask(taskID)
		if t != nil && (t.Status == "Downloading" || t.Status == "Connecting" || t.Status == "Queued") {
			t.Pause()
		}
	}
	return nil
}

// ReorderQueue updates the prioritized order of tasks within a queue.
func (qm *QueueManager) ReorderQueue(id string, newOrder []string) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	q, exists := qm.queues[id]
	if !exists {
		return fmt.Errorf("queue not found")
	}

	q.Order = newOrder
	qm.persist()
	return nil
}

// MoveTask reassigns a task from its current queue to target queue.
func (qm *QueueManager) MoveTask(taskID, targetQueueID string) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	targetQ, exists := qm.queues[targetQueueID]
	if !exists {
		return fmt.Errorf("target queue not found")
	}

	// Remove task from old queue
	for _, q := range qm.queues {
		newOrder := make([]string, 0, len(q.Order))
		for _, id := range q.Order {
			if id != taskID {
				newOrder = append(newOrder, id)
			}
		}
		q.Order = newOrder
	}

	// Append to target queue
	targetQ.Order = append(targetQ.Order, taskID)
	t := qm.engine.GetTask(taskID)
	if t != nil {
		t.QueueID = targetQueueID
		qm.engine.updateTask(t)
	}

	qm.persist()
	return nil
}

// AssignTask ensures a newly created task is registered in its assigned queue.
func (qm *QueueManager) AssignTask(taskID, queueID string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if queueID == "" {
		queueID = "main_queue"
	}

	q, exists := qm.queues[queueID]
	if !exists {
		q = qm.queues["main_queue"]
	}

	if q != nil {
		if q.ID == "main_queue" || !q.ScheduleEnabled {
			q.Running = true
		}
		// Check if already in order
		for _, existing := range q.Order {
			if existing == taskID {
				return
			}
		}
		q.Order = append(q.Order, taskID)
		qm.persist()
	}
}

// RemoveTask purges a deleted task from all queues.
func (qm *QueueManager) RemoveTask(taskID string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	for _, q := range qm.queues {
		newOrder := make([]string, 0, len(q.Order))
		for _, id := range q.Order {
			if id != taskID {
				newOrder = append(newOrder, id)
			}
		}
		q.Order = newOrder
	}
	qm.persist()
}

// CheckQueueProgression is called when any download completes to start next queued tasks.
func (qm *QueueManager) CheckQueueProgression(completedTask *Task) {
	qm.mu.RLock()
	q := qm.queues[completedTask.QueueID]
	qm.mu.RUnlock()

	if q == nil || !q.Running {
		return
	}

	// Check if all tasks in this queue are completed
	allDone := true
	for _, taskID := range q.Order {
		t := qm.engine.GetTask(taskID)
		if t != nil && t.Status != "Completed" {
			allDone = false
			break
		}
	}

	if allDone && len(q.Order) > 0 {
		qm.triggerOnComplete(q)
	}

	qm.engine.checkQueue()
}

func (qm *QueueManager) triggerOnComplete(q *QueueRecord) {
	switch q.OnComplete {
	case "sleep":
		cmd := exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Run()
	case "shutdown":
		cmd := exec.Command("shutdown.exe", "/s", "/t", "30", "/c", "IFD Queue Finished. Shutting down PC in 30 seconds.")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Run()
	case "close_ifd":
		// Gracefully terminate application
		cmd := exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", syscall.Getpid()))
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Run()
	}
}

// scheduleLoop checks time-based start and stop conditions every 30 seconds.
func (qm *QueueManager) scheduleLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-qm.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			curTime := now.Format("15:04")

			qm.mu.RLock()
			for _, q := range qm.queues {
				if !q.ScheduleEnabled {
					continue
				}

				if q.StartTime != "" && curTime == q.StartTime && !q.Running {
					go func(qid string) {
						_ = qm.StartQueue(qid)
					}(q.ID)
				}

				if q.StopTime != "" && curTime == q.StopTime && q.Running {
					go func(qid string) {
						_ = qm.StopQueue(qid)
					}(q.ID)
				}
			}
			qm.mu.RUnlock()
		}
	}
}

// Close stops background schedule goroutines.
func (qm *QueueManager) Close() {
	close(qm.stopCh)
}
