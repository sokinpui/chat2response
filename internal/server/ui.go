package server

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sokinpui/chat2response/internal/utils"
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
	t.ticker = time.NewTicker(150 * time.Millisecond)
	go func() {
		for {
			select {
			case <-t.ticker.C:
				t.draw()
			case <-t.stopChan:
				t.ticker.Stop()
				return
			}
		}
	}()
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
	if len(t.active) == 0 {
		fmt.Print("\r\033[K")
	}
}

func (t *RequestTracker) WriteLog(msg string) {
	fmt.Printf("\r\033[K%s\n", msg)
}

func (t *RequestTracker) draw() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.active) == 0 {
		return
	}

	keys := make([]string, 0, len(t.active))
	for k := range t.active {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		req := t.active[k]
		elapsed := time.Since(req.startTime)
		parts = append(parts, fmt.Sprintf("[%s]", fmtDuration(elapsed)))
	}

	fmt.Printf("\r\033[K⏳ %s", strings.Join(parts, ", "))
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
