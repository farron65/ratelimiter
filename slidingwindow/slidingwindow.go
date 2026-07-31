package slidingwindow

import (
	"sync"
	"time"
)

type SlidingWindowCounter struct {
	windowSize int64
	limit      int64

	windowID int64

	prevCount int64
	currCount int64

	now func() time.Time

	mu sync.Mutex
}

func NewSlidingWindowCounter(windowSize int64, limit int64) *SlidingWindowCounter {
	return &SlidingWindowCounter {
		windowSize: windowSize,
		limit:      limit,

		windowID: time.Now().Unix() / windowSize,

		prevCount: 0,
		currCount: 0,

		now: time.Now,
	}
}

func (swc *SlidingWindowCounter) SetClock(fn func() time.Time) {
	swc.now = fn
}

func (swc *SlidingWindowCounter) Allow() bool {

	swc.mu.Lock()
	defer swc.mu.Unlock()

	now := swc.now().Unix()

	currWindowID := now / swc.windowSize

	if swc.windowID != currWindowID {

		swc.windowID = currWindowID

		swc.prevCount = swc.currCount
		swc.currCount = 0

	}

	elapsed := float64(now) - (float64(swc.windowID) * float64(swc.windowSize))
	weight := ((float64(swc.windowSize) - elapsed) / float64(swc.windowSize))

	estimatedCount := weight * float64(swc.prevCount) + float64(swc.currCount)

	if estimatedCount < float64(swc.limit) {
		swc.currCount++
		return true
	}

	return false
}
