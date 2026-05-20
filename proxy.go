package chat2response

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/sokinpui/chat2response/pkg/proxy"
	"github.com/sokinpui/chat2response/pkg/types"
)

func NewProxyHandler(opts proxy.ProxyOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !proxy.IsResponsesEndpoint(r) || r.Method != http.MethodPost {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "Read body failed")
			return
		}

		var req types.ResponsesRequest
		if err := json.Unmarshal(body, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if opts.Model != "" {
			req.Model = opts.Model
		}

		if opts.DefaultHeaders == nil {
			opts.DefaultHeaders = make(map[string]string)
		}
		for k, v := range proxy.FilterHeaders(r.Header) {
			if len(v) > 0 {
				opts.DefaultHeaders[k] = v[0]
			}
		}

		resp, err := proxy.HandleResponses(r.Context(), req, opts)
		if err != nil {
			jsonError(w, http.StatusBadGateway, err.Error())
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			for _, val := range v {
				w.Header().Add(k, val)
			}
		}

		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"message": message,
			"type":    "proxy_error",
		},
	})
}
