package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sokinpui/chat2response/internal/proxy"
	"github.com/sokinpui/chat2response/internal/types"
)

type Server struct {
	opts    proxy.ProxyOptions
	host    string
	port    int
	cors    bool
	tracker *RequestTracker
}

func NewServer(opts proxy.ProxyOptions, host string, port int, cors bool) *Server {
	return &Server{
		opts:    opts,
		host:    host,
		port:    port,
		cors:    cors,
		tracker: NewRequestTracker(),
	}
}

func (s *Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		fmt.Printf("Proxy listening on http://%s\n", addr)
		fmt.Printf("Upstream format: %s\n", s.opts.UpstreamFormat)
		fmt.Printf("Upstream URL: %s\n", s.opts.BaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Listen error: %v\n", err)
			os.Exit(1)
		}
	}()

	s.tracker.Start()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("\nShutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	if s.cors {
		setCorsHeaders(w)
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if !proxy.IsResponsesEndpoint(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "Not found"}})
		return
	}

	id := s.tracker.Add(r.Method, r.URL.Path)
	defer s.tracker.Remove(id)

	body, _ := io.ReadAll(r.Body)
	var req types.ResponsesRequest
	json.Unmarshal(body, &req)

	if s.opts.Model != "" {
		req.Model = s.opts.Model
	}

	start := time.Now()
	opts := s.opts
	if opts.DefaultHeaders == nil {
		opts.DefaultHeaders = make(map[string]string)
	}
	for k, v := range proxy.FilterHeaders(r.Header) {
		opts.DefaultHeaders[k] = v[0]
	}

	var resultLog string
	opts.OnCacheStats = func(stats types.CacheStats) {
		duration := time.Since(start)
		billed := stats.InputTokens + stats.OutputTokens - stats.CachedTokens
		logMsg := fmt.Sprintf("[%s] -> 200 (%s) [total=%d, input=%d, output=%d, cached=%d, billed=%d]",
			fmtTime(time.Now()),
			fmtDuration(duration),
			stats.TotalTokens, stats.InputTokens, stats.OutputTokens, stats.CachedTokens, billed,
		)
		if stats.CachedTokens < 1024 && billed > 0 {
			resultLog = "⚠️ NO CACHE -- " + logMsg
		} else {
			resultLog = logMsg
		}
	}

	ctx := r.Context()
	if s.opts.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(s.opts.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	resp, err := proxy.HandleResponses(ctx, req, opts)

	if err != nil {
		s.tracker.WriteLog(fmt.Sprintf("Error: %v", err))
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": err.Error()}})
		return
	}
	defer resp.Body.Close()

	// Capture response for debug/last-message
	var respBody bytes.Buffer
	tee := io.TeeReader(resp.Body, &respBody)

	for k, v := range resp.Header {
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}
	if s.cors {
		for k, v := range corsHeaders() {
			w.Header().Set(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, tee)

	if resultLog != "" {
		s.tracker.WriteLog(resultLog)
	} else {
		s.tracker.WriteLog(fmt.Sprintf("[%s] -> %d (%s)", fmtTime(time.Now()), resp.StatusCode, fmtDuration(time.Since(start))))
	}

	// Save last message
	saveLastMessage(req, respBody.Bytes())

	if resp.StatusCode >= 400 {
		dumpPath := saveErrorDump(r, body, resp, respBody.Bytes())
		fmt.Printf("\n[proxy-failure] full exchange saved to %s\n", dumpPath)
	}
}

func setCorsHeaders(w http.ResponseWriter) {
	for k, v := range corsHeaders() {
		w.Header().Set(k, v)
	}
}

func corsHeaders() map[string]string {
	return map[string]string{
		"Access-Control-Allow-Origin":   "*",
		"Access-Control-Allow-Methods":  "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers":  "Authorization, Content-Type, x-api-key, anthropic-version, anthropic-beta, anthropic-dangerous-direct-browser-access",
		"Access-Control-Expose-Headers": "Content-Type",
	}
}
