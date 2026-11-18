package api

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultLogPageSize = 100
	maxLogPageSize     = 500
)

// requestLogEntry is the sanitized representation exposed by the admin API.
// It never contains headers, bodies, tokens or query strings.
type requestLogEntry struct {
	ID         string `json:"id"`
	Time       string `json:"time"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	Bytes      int    `json:"bytes"`
	DurationMs int64  `json:"durationMs"`
	IP         string `json:"ip"`
}

// requestLogRing is a fixed-capacity, concurrency-safe request log. The zero
// value is not usable; create it with newRequestLogRing.
type requestLogRing struct {
	mu       sync.RWMutex
	entries  []requestLogEntry
	head     int
	size     int
	capacity int
}

func newRequestLogRing(capacity int) *requestLogRing {
	if capacity < 1 {
		capacity = defaultLogRetention
	}
	return &requestLogRing{
		entries:  make([]requestLogEntry, capacity),
		capacity: capacity,
	}
}

func (r *requestLogRing) Add(entry requestLogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[r.head] = entry
	r.head = (r.head + 1) % r.capacity
	if r.size < r.capacity {
		r.size++
	}
}

// Resize keeps the newest entries when the configured retention changes.
func (r *requestLogRing) Resize(capacity int) {
	if capacity < 1 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if capacity == r.capacity {
		return
	}
	newest := make([]requestLogEntry, r.size)
	for i := range newest {
		idx := (r.head - 1 - i + r.capacity) % r.capacity
		newest[i] = r.entries[idx]
	}
	keep := r.size
	if keep > capacity {
		keep = capacity
	}
	entries := make([]requestLogEntry, capacity)
	for i := 0; i < keep; i++ {
		entries[i] = newest[keep-1-i]
	}
	r.entries = entries
	r.capacity = capacity
	r.size = keep
	r.head = keep % capacity
}

// Snapshot returns entries ordered newest first.
func (r *requestLogRing) Snapshot() []requestLogEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]requestLogEntry, 0, r.size)
	for i := 0; i < r.size; i++ {
		idx := (r.head - 1 - i + r.capacity) % r.capacity
		out = append(out, r.entries[idx])
	}
	return out
}

// Query applies status and substring filters to the newest-first snapshot.
// A negative status disables status filtering.
func (r *requestLogRing) Query(status int, q string, limit int) (items []requestLogEntry, total int, truncated bool) {
	all := r.Snapshot()
	items = make([]requestLogEntry, 0, len(all))
	needle := strings.ToLower(q)
	for _, entry := range all {
		if status >= 0 && entry.Status != status {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(entry.Method+entry.Path+entry.IP), needle) {
			continue
		}
		items = append(items, entry)
	}
	total = len(items)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
		truncated = true
	}
	return items, total, truncated
}

func (s *Server) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	query := r.URL.Query()
	status := -1
	if raw := query.Get("status"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_status", "status must be an integer", nil)
			return
		}
		status = n
	}
	limit, err := parseLimit(query.Get("limit"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if limit == 0 {
		limit = defaultLogPageSize
	} else if limit > maxLogPageSize {
		writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 500", nil)
		return
	}
	items, total, truncated := s.logs.Query(status, query.Get("q"), limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"total":     total,
		"truncated": truncated,
	})
}

func (s *Server) recordRequestLog(r *http.Request, status, bytes int, duration time.Duration) {
	entry := requestLogEntry{
		ID:         newRequestID(),
		Time:       time.Now().UTC().Format(time.RFC3339),
		Method:     r.Method,
		Path:       r.URL.Path,
		Status:     status,
		Bytes:      bytes,
		DurationMs: duration.Milliseconds(),
		IP:         clientIP(r, s.cfg.TrustProxy),
	}
	s.logs.Add(entry)
}
