package engine

import (
	"errors"
	"sync"
)

const (
	// MinSubdivideSize is the minimum remaining bytes required in a segment to allow splitting (1 MB)
	MinSubdivideSize = 1024 * 1024
)

// Segment represents a contiguous byte range assigned to a download connection.
type Segment struct {
	ID            int    `json:"id"`
	StartOffset   int64  `json:"startOffset"`
	CurrentOffset int64  `json:"currentOffset"`
	EndOffset     int64  `json:"endOffset"` // inclusive; -1 means unknown/unbounded
	Speed         int64  `json:"speed"`     // bytes/sec
	Active        bool   `json:"active"`
	Done          bool   `json:"done"`
	Error         string `json:"error,omitempty"`
}

// DownloadedBytes returns total bytes downloaded by this segment so far.
func (s *Segment) DownloadedBytes() int64 {
	if s.CurrentOffset < s.StartOffset {
		return 0
	}
	return s.CurrentOffset - s.StartOffset
}

// TotalBytes returns the total span of this segment, or -1 if unbounded.
func (s *Segment) TotalBytes() int64 {
	if s.EndOffset < 0 {
		return -1
	}
	return s.EndOffset - s.StartOffset + 1
}

// RemainingBytes returns bytes remaining for this segment.
func (s *Segment) RemainingBytes() int64 {
	if s.EndOffset < 0 {
		return -1
	}
	rem := s.EndOffset - s.CurrentOffset + 1
	if rem < 0 {
		return 0
	}
	return rem
}

// SegmentManager coordinates segment division and dynamic reallocation.
type SegmentManager struct {
	mu        sync.RWMutex
	segments  []*Segment
	nextID    int
	totalSize int64
}

// NewSegmentManager creates a manager for a file of totalSize.
func NewSegmentManager(totalSize int64, count int) *SegmentManager {
	if count < 1 {
		count = 8
	}
	if count > 32 {
		count = 32
	}

	sm := &SegmentManager{
		totalSize: totalSize,
		nextID:    0,
	}

	if totalSize <= 0 {
		// Single progressive stream
		sm.segments = []*Segment{
			{
				ID:            0,
				StartOffset:   0,
				CurrentOffset: 0,
				EndOffset:     -1,
				Active:        false,
				Done:          false,
			},
		}
		sm.nextID = 1
		return sm
	}

	// For tiny files, reduce segment count
	if totalSize < int64(count)*MinSubdivideSize {
		count = int(totalSize / MinSubdivideSize)
		if count < 1 {
			count = 1
		}
	}

	chunkSize := totalSize / int64(count)
	sm.segments = make([]*Segment, count)

	for i := 0; i < count; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize - 1
		if i == count-1 {
			end = totalSize - 1 // Last segment takes the remainder
		}

		sm.segments[i] = &Segment{
			ID:            i,
			StartOffset:   start,
			CurrentOffset: start,
			EndOffset:     end,
			Active:        false,
			Done:          false,
		}
	}

	sm.nextID = count
	return sm
}

// GetAll returns a snapshot copy of all segments.
func (sm *SegmentManager) GetAll() []*Segment {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	res := make([]*Segment, len(sm.segments))
	for i, seg := range sm.segments {
		copySeg := *seg
		res[i] = &copySeg
	}
	return res
}

// Segments returns the direct references to active segments.
func (sm *SegmentManager) Segments() []*Segment {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	res := make([]*Segment, len(sm.segments))
	copy(res, sm.segments)
	return res
}

// TotalDownloaded calculates sum of all downloaded bytes across all segments.
func (sm *SegmentManager) TotalDownloaded() int64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var sum int64
	for _, s := range sm.segments {
		sum += s.DownloadedBytes()
	}
	return sum
}

// IsComplete returns true if all segments are done.
func (sm *SegmentManager) IsComplete() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, s := range sm.segments {
		if !s.Done {
			return false
		}
	}
	return len(sm.segments) > 0
}

// SubdivideLargest finds the active segment with the largest remaining bytes
// and splits its unfinished tail into a new segment for an idle worker.
func (sm *SegmentManager) SubdivideLargest() (*Segment, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.totalSize <= 0 {
		return nil, errors.New("cannot subdivide unbounded stream")
	}

	var bestSeg *Segment
	var maxRemaining int64 = 0

	for _, s := range sm.segments {
		if s.Active && !s.Done {
			rem := s.EndOffset - s.CurrentOffset + 1
			if rem > maxRemaining {
				maxRemaining = rem
				bestSeg = s
			}
		}
	}

	if bestSeg == nil || maxRemaining < MinSubdivideSize*2 {
		return nil, errors.New("no segment eligible for dynamic subdivision")
	}

	// Split at midpoint of remaining range
	mid := bestSeg.CurrentOffset + (maxRemaining / 2)
	oldEnd := bestSeg.EndOffset

	// Update existing segment's boundary
	bestSeg.EndOffset = mid - 1

	// Create new segment taking over the second half
	newSeg := &Segment{
		ID:            sm.nextID,
		StartOffset:   mid,
		CurrentOffset: mid,
		EndOffset:     oldEnd,
		Active:        true,
		Done:          false,
	}
	sm.nextID++
	sm.segments = append(sm.segments, newSeg)

	return newSeg, nil
}
