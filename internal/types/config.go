package types

type UpstreamFormat string

const (
	UpstreamFormatAnthropic  UpstreamFormat = "anthropic"
	UpstreamFormatOpenAIChat UpstreamFormat = "openai-chat"
)

type UpstreamConfig struct {
	Format          UpstreamFormat    `json:"format,omitempty"`
	BaseURL         string            `json:"baseUrl"`
	Host            *string           `json:"host,omitempty"`
	Port            any               `json:"port,omitempty"` // string | number
	APIVersion      *string           `json:"apiVersion,omitempty"`
	APIKey          *string           `json:"apiKey,omitempty"`
	Model           *string           `json:"model,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	TimeoutMs       *int              `json:"timeoutMs,omitempty"`
	DropImages      *bool             `json:"dropImages,omitempty"`
	Fallback        *string           `json:"fallback,omitempty"`
	ReasoningEffort *string           `json:"reasoningEffort,omitempty"`
	Thinking        any               `json:"thinking,omitempty"`
}

type ConfigFile struct {
	Version         string                    `json:"version"`
	CurrentUpstream string                    `json:"currentUpstream"`
	Headers         map[string]string         `json:"headers,omitempty"`
	Upstreams       map[string]UpstreamConfig `json:"upstreams"`
	ReasoningEffort *string                   `json:"reasoningEffort,omitempty"`
	Thinking        any                       `json:"thinking,omitempty"`
	TimeoutMs       *int                      `json:"timeoutMs,omitempty"`
}

var ConfigFileNames = []string{
	"codeproxy.config.json",
	"codeproxy.config.js",
	"codeproxy.config.mjs",
	"codeproxy.config.ts",
	".codeproxy.json",
	".codeproxy.js",
}
