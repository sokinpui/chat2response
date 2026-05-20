package utils

import (
	"fmt"
	"sync"
	"time"
)

var (
	counter int64
	mu      sync.Mutex
)

func NowMs() int64 {
	return time.Now().UnixMilli()
}

func NextSeq() int64 {
	mu.Lock()
	defer mu.Unlock()
	counter = (counter + 1) & 0x7fffffff
	return counter
}

func MakeId(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, NowMs(), NextSeq())
}
