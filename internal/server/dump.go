package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func saveLastMessage(req any, resp []byte) {
	dir := "logs"
	os.MkdirAll(dir, 0755)

	payload := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"request":   req,
		"response":  tryParseJson(resp),
	}

	data, _ := json.MarshalIndent(payload, "", "  ")
	os.WriteFile(filepath.Join(dir, "last-message.json"), data, 0644)
}

func saveErrorDump(r *http.Request, reqBody []byte, resp *http.Response, respBody []byte) string {
	dir := "logs"
	os.MkdirAll(dir, 0755)

	ts := time.Now().Format("2006-01-02T15-04-05")
	filename := fmt.Sprintf("proxy-error-%s-%d.json", ts, resp.StatusCode)
	path := filepath.Join(dir, filename)

	clientHeaders := make(map[string]string)
	for k, v := range r.Header {
		clientHeaders[k] = strings.Join(v, ", ")
	}
	redactAuth(clientHeaders)

	dump := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"method":    r.Method,
		"url":       r.URL.String(),
		"clientRequest": map[string]any{
			"headers": clientHeaders,
			"body":    tryParseJson(reqBody),
		},
		"proxyResponse": map[string]any{
			"status":  resp.StatusCode,
			"headers": resp.Header,
			"body":    tryParseJson(respBody),
		},
	}

	data, _ := json.MarshalIndent(dump, "", "  ")
	os.WriteFile(path, data, 0644)
	return path
}

func redactAuth(headers map[string]string) {
	for k := range headers {
		lk := strings.ToLower(k)
		if lk == "authorization" || lk == "x-api-key" || lk == "api-key" || lk == "cookie" {
			headers[k] = "[REDACTED]"
		}
	}
}

func tryParseJson(data []byte) any {
	var v any
	if err := json.Unmarshal(data, &v); err == nil {
		return v
	}
	return string(data)
}
