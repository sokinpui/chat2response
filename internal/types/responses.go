package types

const (
	ResponsesItemTypeMessage               = "message"
	ResponsesItemTypeReasoning             = "reasoning"
	ResponsesItemTypeFunctionCall          = "function_call"
	ResponsesItemTypeFunctionCallOutput    = "function_call_output"
	ResponsesItemTypeLocalShellCall        = "local_shell_call"
	ResponsesItemTypeCommandExecution      = "commandExecution"
	ResponsesItemTypeCommandExecutionOutput = "commandExecutionOutput"
	ResponsesItemTypeCustomToolCall        = "custom_tool_call"
	ResponsesItemTypeCustomToolCallOutput  = "custom_tool_call_output"
	ResponsesItemTypeFileChange            = "fileChange"
	ResponsesItemTypeFileChangeOutput      = "fileChangeOutput"
	ResponsesItemTypeWebSearchCall         = "web_search_call"
)

type ResponsesContentPart struct {
	Type             string                 `json:"type"`
	Text             *string                `json:"text,omitempty"`
	ImageURL         interface{}            `json:"image_url,omitempty"` // string | { url: string }
	Source           map[string]interface{} `json:"source,omitempty"`
	Data             *string                `json:"data,omitempty"`
	Base64           *string                `json:"base64,omitempty"`
	MediaType        *string                `json:"media_type,omitempty"`
	MimeType         *string                `json:"mime_type,omitempty"`
	FileData         *string                `json:"file_data,omitempty"`
	FileURL          interface{}            `json:"file_url,omitempty"`
	ToolUseID        *string                `json:"tool_use_id,omitempty"`
	CallID           *string                `json:"call_id,omitempty"`
	Content          interface{}            `json:"content,omitempty"`
	CacheControl     map[string]interface{} `json:"cache_control,omitempty"`
	Extra            map[string]interface{} `json:"-"`
}

type ResponsesMessageItem struct {
	Type             *string                `json:"type,omitempty"`
	Role             *string                `json:"role,omitempty"`
	Content          interface{}            `json:"content,omitempty"`
	ReasoningContent *string                `json:"reasoning_content,omitempty"`
	ThoughtSignature *string                `json:"thought_signature,omitempty"`
	Extra            map[string]interface{} `json:"-"`
}

type ResponsesReasoningItem struct {
	Type             string                 `json:"type"`
	Content          interface{}            `json:"content,omitempty"`
	ThoughtSignature *string                `json:"thought_signature,omitempty"`
	Extra            map[string]interface{} `json:"-"`
}

type ResponsesFunctionCallItem struct {
	Type      string                 `json:"type"`
	ID        *string                `json:"id,omitempty"`
	CallID    *string                `json:"call_id,omitempty"`
	Name      *string                `json:"name,omitempty"`
	Arguments interface{}            `json:"arguments,omitempty"`
	Extra     map[string]interface{} `json:"-"`
}

type ResponsesTool struct {
	Type        string                 `json:"type"`
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Strict      *bool                  `json:"strict,omitempty"`
	Function    *struct {
		Name        string                 `json:"name"`
		Description *string                `json:"description,omitempty"`
		Parameters  map[string]interface{} `json:"parameters,omitempty"`
		Strict      *bool                  `json:"strict,omitempty"`
	} `json:"function,omitempty"`
	Extra map[string]interface{} `json:"-"`
}

type ResponsesReasoningConfig struct {
	Effort  *string                `json:"effort,omitempty"` // "minimal" | "low" | "medium" | "high" | "xhigh"
	Summary *string                `json:"summary,omitempty"`
	Extra   map[string]interface{} `json:"-"`
}

type ResponsesRequest struct {
	Model              string                   `json:"model"`
	Input              interface{}              `json:"input,omitempty"`        // ResponsesInputItem[] | string
	Instructions       interface{}              `json:"instructions,omitempty"` // string | Array
	Tools              []ResponsesTool          `json:"tools,omitempty"`
	ToolChoice         interface{}              `json:"tool_choice,omitempty"`
	Temperature        *float64                 `json:"temperature,omitempty"`
	TopP               *float64                 `json:"top_p,omitempty"`
	MaxOutputTokens    *int                     `json:"max_output_tokens,omitempty"`
	MaxTokens          *int                     `json:"max_tokens,omitempty"`
	Stream             *bool                    `json:"stream,omitempty"`
	Reasoning          *ResponsesReasoningConfig `json:"reasoning,omitempty"`
	Metadata           map[string]interface{}   `json:"metadata,omitempty"`
	Store              *bool                    `json:"store,omitempty"`
	PreviousResponseID *string                  `json:"previous_response_id,omitempty"`
	Extra              map[string]interface{}   `json:"-"`
}

type ResponsesOutputMessage struct {
	ID      string                   `json:"id"`
	Type    string                   `json:"type"` // "message"
	Role    string                   `json:"role"` // "assistant"
	Status  string                   `json:"status"`
	Content []map[string]interface{} `json:"content"`
}

type ResponsesOutputFunctionCall struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	Status    string  `json:"status"`
	Name      *string `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitempty"`
	CallID    *string `json:"call_id,omitempty"`
	Action    *struct {
		Type    *string  `json:"type,omitempty"`
		Command []string `json:"command,omitempty"`
	} `json:"action,omitempty"`
}

type ResponsesOutputReasoning struct {
	ID      string                   `json:"id"`
	Type    string                   `json:"type"` // "reasoning"
	Summary []interface{}            `json:"summary"`
	Content []ResponsesContentPart   `json:"content"`
	Status  *string                  `json:"status,omitempty"`
}

type ResponsesUsage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	TotalTokens         int `json:"total_tokens"`
	InputTokensDetails *struct {
		CachedTokens        *int `json:"cached_tokens,omitempty"`
		CacheCreationTokens *int `json:"cache_creation_tokens,omitempty"`
	} `json:"input_tokens_details,omitempty"`
}

type ResponsesResponse struct {
	ID                 string                 `json:"id"`
	Object             string                 `json:"object"` // "response"
	CreatedAt          int64                  `json:"created_at"`
	CompletedAt        *int64                 `json:"completed_at,omitempty"`
	Model              string                 `json:"model"`
	Status             string                 `json:"status"`
	Output             []interface{}          `json:"output"`
	Usage              *ResponsesUsage        `json:"usage,omitempty"`
	Temperature        *float64               `json:"temperature,omitempty"`
	TopP               *float64               `json:"top_p,omitempty"`
	ToolChoice         interface{}            `json:"tool_choice,omitempty"`
	Tools              []interface{}          `json:"tools,omitempty"`
	ParallelToolCalls *bool                  `json:"parallel_tool_calls,omitempty"`
	Store              *bool                  `json:"store,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	Extra              map[string]interface{} `json:"-"`
}

type ResponsesStreamEvent struct {
	ID             string                 `json:"id"`
	Object         string                 `json:"object"` // "response.event"
	Type           string                 `json:"type"`
	CreatedAt      int64                  `json:"created_at"`
	SequenceNumber int                    `json:"sequence_number"`
	Extra          map[string]interface{} `json:"-"`
}
