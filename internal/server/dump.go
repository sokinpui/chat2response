package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func printExchange(req any, resp []byte) {
	payload := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"request":   req,
		"response":  tryParseJson(resp),
	}

	data, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Printf("\n--- Exchange Detail ---\n%s\n----------------------\n", string(data))
}

func printErrorDump(r *http.Request, reqBody []byte, resp *http.Response, respBody []byte) {
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
	fmt.Printf("\n--- Proxy Error Dump (%d) ---\n%s\n---------------------------\n", resp.StatusCode, string(data))
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
