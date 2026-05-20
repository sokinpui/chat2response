package types

const (
	AnthropicBlockTypeText       = "text"
	AnthropicBlockTypeImage      = "image"
	AnthropicBlockTypeDocument   = "document"
	AnthropicBlockTypeToolUse    = "tool_use"
	AnthropicBlockTypeToolResult = "tool_result"
	AnthropicBlockTypeThinking   = "thinking"
)

type AnthropicTextBlock struct {
	Type         string         `json:"type"`
	Text         string         `json:"text"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

type AnthropicImageSource struct {
	Type      string  `json:"type"` // "base64" | "url"
	MediaType *string `json:"media_type,omitempty"`
	Data      *string `json:"data,omitempty"`
	URL       *string `json:"url,omitempty"`
}

type AnthropicImageBlock struct {
	Type         string               `json:"type"`
	Source       AnthropicImageSource `json:"source"`
	CacheControl map[string]any       `json:"cache_control,omitempty"`
}

type AnthropicDocumentBlock struct {
	Type         string               `json:"type"`
	Source       AnthropicImageSource `json:"source"`
	CacheControl map[string]any       `json:"cache_control,omitempty"`
}

type AnthropicToolUseBlock struct {
	Type         string         `json:"type"`
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Input        map[string]any `json:"input"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

type AnthropicToolResultBlock struct {
	Type         string         `json:"type"`
	ToolUseID    string         `json:"tool_use_id"`
	Content      any            `json:"content"` // string | AnthropicContentBlock[]
	IsError      *bool          `json:"is_error,omitempty"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

type AnthropicThinkingBlock struct {
	Type      string  `json:"type"`
	Thinking  string  `json:"thinking"`
	Signature *string `json:"signature,omitempty"`
}

type AnthropicContentBlock struct {
	Type         string                `json:"type"`
	Text         *string               `json:"text,omitempty"`
	Source       *AnthropicImageSource `json:"source,omitempty"`
	ID           *string               `json:"id,omitempty"`
	Name         *string               `json:"name,omitempty"`
	Input        map[string]any        `json:"input,omitempty"`
	ToolUseID    *string               `json:"tool_use_id,omitempty"`
	Content      any                   `json:"content,omitempty"`
	IsError      *bool                 `json:"is_error,omitempty"`
	Thinking     *string               `json:"thinking,omitempty"`
	Signature    *string               `json:"signature,omitempty"`
	CacheControl map[string]any        `json:"cache_control,omitempty"`
}

type AnthropicMessage struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content any    `json:"content"` // AnthropicContentBlock[] | string
}

type AnthropicTool struct {
	Name        string         `json:"name"`
	Description *string        `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type AnthropicToolChoice struct {
	Type string `json:"type"` // "auto" | "any" | "tool"
	Name string `json:"name,omitempty"`
}

type AnthropicThinkingConfig struct {
	Type         string `json:"type"` // "enabled" | "disabled"
	BudgetTokens *int   `json:"budget_tokens,omitempty"`
}

type AnthropicRequest struct {
	Model       string                   `json:"model"`
	Messages    []AnthropicMessage       `json:"messages"`
	System      any                      `json:"system,omitempty"` // string | AnthropicTextBlock[]
	MaxTokens   int                      `json:"max_tokens"`
	Temperature *float64                 `json:"temperature,omitempty"`
	TopP        *float64                 `json:"top_p,omitempty"`
	Tools       []any                    `json:"tools,omitempty"`
	ToolChoice  *AnthropicToolChoice     `json:"tool_choice,omitempty"`
	Metadata    map[string]any           `json:"metadata,omitempty"`
	Thinking    *AnthropicThinkingConfig `json:"thinking,omitempty"`
	Stream      *bool                    `json:"stream,omitempty"`
	Extra       map[string]any           `json:"-"`
}

type AnthropicUsage struct {
	InputTokens              int  `json:"input_tokens"`
	OutputTokens             int  `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
}

type AnthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"` // always "message"
	Role         string                  `json:"role"` // always "assistant"
	Model        string                  `json:"model"`
	Content      []AnthropicContentBlock `json:"content"`
	StopReason   *string                 `json:"stop_reason,omitempty"`
	StopSequence *string                 `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage          `json:"usage"`
}

type AnthropicStreamEvent struct {
	Type         string                 `json:"type"`
	Message      *AnthropicResponse     `json:"message,omitempty"`
	Index        *int                   `json:"index,omitempty"`
	ContentBlock *AnthropicContentBlock `json:"content_block,omitempty"`
	Delta        map[string]any         `json:"delta,omitempty"`
	Usage        *AnthropicUsage        `json:"usage,omitempty"`
}
