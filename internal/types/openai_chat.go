package types

const (
	OpenAiRoleSystem    = "system"
	OpenAiRoleUser      = "user"
	OpenAiRoleAssistant = "assistant"
	OpenAiRoleTool      = "tool"
	OpenAiRoleDeveloper = "developer"
)

type OpenAiChatToolCall struct {
	ID       *string `json:"id,omitempty"`
	Type     *string `json:"type,omitempty"` // "function"
	Function *struct {
		Name      *string `json:"name,omitempty"`
		Arguments any     `json:"arguments,omitempty"` // string | map[string]interface{}
	} `json:"function,omitempty"`
}

type OpenAiChatMessage struct {
	Role             string               `json:"role"`
	Content          any                  `json:"content,omitempty"` // string | []map[string]interface{} | nil
	Name             *string              `json:"name,omitempty"`
	ToolCallID       *string              `json:"tool_call_id,omitempty"`
	ToolCalls        []OpenAiChatToolCall `json:"tool_calls,omitempty"`
	ReasoningContent *string              `json:"reasoning_content,omitempty"`
	Extra            map[string]any       `json:"-"`
}

type OpenAiChatFunctionTool struct {
	Type     string `json:"type"` // "function"
	Function struct {
		Name        string         `json:"name"`
		Description *string        `json:"description,omitempty"`
		Parameters  map[string]any `json:"parameters,omitempty"`
	} `json:"function"`
}

type OpenAiChatWebSearchTool struct {
	Type      string `json:"type"` // "web_search"
	WebSearch struct {
		Enable       bool           `json:"enable"`
		SearchEngine *string        `json:"search_engine,omitempty"`
		Extra        map[string]any `json:"-"`
	} `json:"web_search"`
}

type OpenAiChatRequest struct {
	Model       string              `json:"model"`
	Messages    []OpenAiChatMessage `json:"messages"`
	Stream      *bool               `json:"stream,omitempty"`
	Temperature *float64            `json:"temperature,omitempty"`
	TopP        *float64            `json:"top_p,omitempty"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
	Tools       []any               `json:"tools,omitempty"`
	ToolChoice  any                 `json:"tool_choice,omitempty"`
	Extra       map[string]any      `json:"-"`
}

type OpenAiChatUsage struct {
	PromptTokens        *int `json:"prompt_tokens,omitempty"`
	CompletionTokens    *int `json:"completion_tokens,omitempty"`
	TotalTokens         *int `json:"total_tokens,omitempty"`
	PromptTokensDetails *struct {
		CachedTokens *int           `json:"cached_tokens,omitempty"`
		Extra        map[string]any `json:"-"`
	} `json:"prompt_tokens_details,omitempty"`
	Extra map[string]any `json:"-"`
}

type OpenAiChatChoice struct {
	Index   *int `json:"index,omitempty"`
	Message struct {
		Role             *string              `json:"role,omitempty"`
		Content          *string              `json:"content,omitempty"`
		ToolCalls        []OpenAiChatToolCall `json:"tool_calls,omitempty"`
		ReasoningContent *string              `json:"reasoning_content,omitempty"`
		Extra            map[string]any       `json:"-"`
	} `json:"message"`
	FinishReason *string        `json:"finish_reason,omitempty"`
	Extra        map[string]any `json:"-"`
}

type OpenAiChatResponse struct {
	ID      *string            `json:"id,omitempty"`
	Object  *string            `json:"object,omitempty"`
	Created *int64             `json:"created,omitempty"`
	Model   *string            `json:"model,omitempty"`
	Choices []OpenAiChatChoice `json:"choices"`
	Usage   *OpenAiChatUsage   `json:"usage,omitempty"`
	Extra   map[string]any     `json:"-"`
}

type OpenAiChatStreamDeltaToolCall struct {
	Index    int     `json:"index"`
	ID       *string `json:"id,omitempty"`
	Type     *string `json:"type,omitempty"`
	Function *struct {
		Name      *string `json:"name,omitempty"`
		Arguments any     `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

type OpenAiChatStreamDelta struct {
	Role             *string                         `json:"role,omitempty"`
	Content          *string                         `json:"content,omitempty"`
	ReasoningContent *string                         `json:"reasoning_content,omitempty"`
	ToolCalls        []OpenAiChatStreamDeltaToolCall `json:"tool_calls,omitempty"`
	Extra            map[string]any                  `json:"-"`
}

type OpenAiChatStreamChoice struct {
	Index        *int                   `json:"index,omitempty"`
	Delta        *OpenAiChatStreamDelta `json:"delta,omitempty"`
	FinishReason *string                `json:"finish_reason,omitempty"`
	Extra        map[string]any         `json:"-"`
}

type OpenAiChatStreamChunk struct {
	ID      *string                  `json:"id,omitempty"`
	Object  *string                  `json:"object,omitempty"`
	Created *int64                   `json:"created,omitempty"`
	Model   *string                  `json:"model,omitempty"`
	Choices []OpenAiChatStreamChoice `json:"choices,omitempty"`
	Usage   *OpenAiChatUsage         `json:"usage,omitempty"`
	Extra   map[string]any           `json:"-"`
}
