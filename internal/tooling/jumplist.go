package tooling

import "sync"

// Location represents a specific file and cursor coordinate.
type Location struct {
	FilePath string
	Line     int // 0-indexed line
	Col      int // 0-indexed col
}

// Jumplist manages a cross-file navigation history stack (Ctrl+o / Ctrl+i).
type Jumplist struct {
	mu        sync.RWMutex
	locations []Location
	current   int
	maxSize   int
}

// NewJumplist creates a new Jumplist with the specified maximum capacity.
func NewJumplist(maxSize int) *Jumplist {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &Jumplist{
		locations: make([]Location, 0, maxSize),
		current:   -1,
		maxSize:   maxSize,
	}
}

// Record appends a new location to the history. If the current pointer is
// not at the end of the history, subsequent forward history is truncated.
// Duplicate adjacent records are ignored.
func (j *Jumplist) Record(loc Location) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if len(j.locations) > 0 && j.current >= 0 && j.current < len(j.locations) {
		curr := j.locations[j.current]
		if curr.FilePath == loc.FilePath && curr.Line == loc.Line && curr.Col == loc.Col {
			return // Avoid recording identical adjacent position
		}
		// Truncate forward history if recording after moving back
		j.locations = j.locations[:j.current+1]
	}

	j.locations = append(j.locations, loc)
	if len(j.locations) > j.maxSize {
		j.locations = j.locations[1:]
	}
	j.current = len(j.locations) - 1
}

// Back moves the pointer backward in history and returns the target location.
func (j *Jumplist) Back() (Location, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.current > 0 {
		j.current--
		return j.locations[j.current], true
	}
	return Location{}, false
}

// Forward moves the pointer forward in history and returns the target location.
func (j *Jumplist) Forward() (Location, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.current >= 0 && j.current < len(j.locations)-1 {
		j.current++
		return j.locations[j.current], true
	}
	return Location{}, false
}

// Current returns the location currently pointed to by the history pointer.
func (j *Jumplist) Current() (Location, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if j.current >= 0 && j.current < len(j.locations) {
		return j.locations[j.current], true
	}
	return Location{}, false
}

// Len returns the number of recorded locations.
func (j *Jumplist) Len() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return len(j.locations)
}

// Clear resets the jumplist history.
func (j *Jumplist) Clear() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.locations = j.locations[:0]
	j.current = -1
}
