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

	"github.com/sokinpui/chat2response/pkg/proxy"
	"github.com/sokinpui/chat2response/pkg/types"
)

type Server struct {
	opts    proxy.ProxyOptions
	host    string
	port    int
	cors    bool
	logLevel string
	tracker *RequestTracker
}

func NewServer(opts proxy.ProxyOptions, host string, port int, cors bool, logLevel string) *Server {
	return &Server{
		opts:     opts,
		host:     host,
		port:     port,
		cors:     cors,
		logLevel: logLevel,
		tracker:  NewRequestTracker(),
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

	s.tracker.WriteLog(fmt.Sprintf("[%s] <- %s %s", fmtTime(time.Now()), r.Method, r.URL.Path))

	body, _ := io.ReadAll(r.Body)
	var req types.ResponsesRequest
	json.Unmarshal(body, &req)

	if s.opts.Model != "" {
		req.Model = s.opts.Model
	}

	opts := s.opts
	if opts.DefaultHeaders == nil {
		opts.DefaultHeaders = make(map[string]string)
	}
	for k, v := range proxy.FilterHeaders(r.Header) {
		opts.DefaultHeaders[k] = v[0]
	}

	format := proxy.ResolveFormat(opts)
	targetUrl := proxy.NormalizeBaseUrl(s.opts.BaseURL, format)

	s.tracker.WriteLog(fmt.Sprintf("[%s] -> redirect to Upstream POST %s", fmtTime(time.Now()), targetUrl))

	var resultLog string
	opts.OnCacheStats = func(stats types.CacheStats) {
		if s.logLevel != "debug" {
			return
		}
		billed := stats.InputTokens + stats.OutputTokens - stats.CachedTokens
		logMsg := fmt.Sprintf("   -> Cache Stats: [total=%d, input=%d, output=%d, cached=%d, billed=%d]",
			fmtTime(time.Now()),
			stats.TotalTokens, stats.InputTokens, stats.OutputTokens, stats.CachedTokens, billed,
		)
		if stats.CachedTokens < 1024 && billed > 0 {
			resultLog = "NO CACHE -- " + logMsg
		} else {
			resultLog = logMsg
		}
	}

	ctx := r.Context()

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

	s.tracker.WriteLog(fmt.Sprintf("[%s] <- %d %s from Upstream %s", fmtTime(time.Now()), resp.StatusCode, http.StatusText(resp.StatusCode), targetUrl))

	if resultLog != "" {
		s.tracker.WriteLog(resultLog)
	}

	s.tracker.WriteLog(fmt.Sprintf("[%s] <- redirect from Upstream %s to client", fmtTime(time.Now()), targetUrl))

	if s.logLevel == "debug" {
		printExchange(req, respBody.Bytes())

		if resp.StatusCode >= 400 {
			printErrorDump(r, body, resp, respBody.Bytes())
		}
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
