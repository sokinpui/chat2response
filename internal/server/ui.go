package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/sokinpui/chat2response/pkg/utils"
)

type activeRequest struct {
	id        string
	method    string
	url       string
	startTime time.Time
}

type RequestTracker struct {
	mu       sync.Mutex
	active   map[string]*activeRequest
	ticker   *time.Ticker
	stopChan chan struct{}
}

func NewRequestTracker() *RequestTracker {
	return &RequestTracker{
		active:   make(map[string]*activeRequest),
		stopChan: make(chan struct{}),
	}
}

func (t *RequestTracker) Start() {
	// Dynamic timer display disabled
}

func (t *RequestTracker) Add(method, url string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	id := utils.MakeId("req")
	t.active[id] = &activeRequest{
		id:        id,
		method:    method,
		url:       url,
		startTime: time.Now(),
	}
	return id
}

func (t *RequestTracker) Remove(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.active, id)
}

func (t *RequestTracker) WriteLog(msg string) {
	fmt.Printf("\r\033[K%s\n", msg)
}

func fmtTime(t time.Time) string {
	return t.Format("15:04:05")
}

func fmtDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	min := int(d.Minutes())
	sec := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", min, sec)
}
