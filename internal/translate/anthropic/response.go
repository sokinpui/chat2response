package anthropic

import (
	"strings"
	"time"

	"github.com/sokinpui/chat2response/internal/types"
	"github.com/sokinpui/chat2response/internal/utils"
)

var shellToolNames = map[string]bool{
	"shell":          true,
	"container.exec": true,
	"shell_command":  true,
}

type TranslateResponseOptions struct {
	ResponseID *string
	CreatedAt  *int64
	Model      *string
}

func TranslateResponse(body types.AnthropicResponse, options TranslateResponseOptions) types.ResponsesResponse {
	createdAt := time.Now().Unix()
	if options.CreatedAt != nil {
		createdAt = *options.CreatedAt
	}

	id := utils.MakeId("resp")
	if body.ID != "" {
		id = body.ID
	}
	if options.ResponseID != nil {
		id = *options.ResponseID
	}

	model := body.Model
	if options.Model != nil {
		model = *options.Model
	}

	output := MapOutputItems(body.Content)

	return types.ResponsesResponse{
		ID:        id,
		Object:    "response",
		CreatedAt: createdAt,
		Model:     model,
		Status:    "completed",
		Output:    output,
		Usage: &types.ResponsesUsage{
			InputTokens:  body.Usage.InputTokens,
			OutputTokens: body.Usage.OutputTokens,
			TotalTokens:  body.Usage.InputTokens + body.Usage.OutputTokens,
			InputTokensDetails: &struct {
				CachedTokens        *int `json:"cached_tokens,omitempty"`
				CacheCreationTokens *int `json:"cache_creation_tokens,omitempty"`
			}{
				CachedTokens:        body.Usage.CacheReadInputTokens,
				CacheCreationTokens: body.Usage.CacheCreationInputTokens,
			},
		},
	}
}

func MapOutputItems(content []types.AnthropicContentBlock) []any {
	var out []any
	var textChunks []string

	for _, block := range content {
		switch block.Type {
		case "text":
			if block.Text != nil {
				textChunks = append(textChunks, *block.Text)
			}
		case "tool_use":
			callID := utils.MakeId("call")
			if block.ID != nil {
				callID = *block.ID
			}
			name := "tool"
			if block.Name != nil {
				name = *block.Name
			}
			args := utils.JsonStringifySafe(block.Input)
			item := types.ResponsesOutputFunctionCall{
				ID:        callID,
				Type:      types.ResponsesItemTypeFunctionCall,
				Status:    "completed",
				Name:      &name,
				Arguments: &args,
				CallID:    &callID,
			}
			if shellToolNames[name] {
				item.Type = types.ResponsesItemTypeLocalShellCall
				var cmd []string
				if arr, ok := block.Input["command"].([]any); ok {
					for _, v := range arr {
						if s, ok := v.(string); ok {
							cmd = append(cmd, s)
						}
					}
				}
				item.Action = &struct {
					Type    *string  `json:"type,omitempty"`
					Command []string `json:"command,omitempty"`
				}{
					Type:    new("exec"),
					Command: cmd,
				}
			}
			out = append(out, item)
		case "thinking":
			text := ""
			if block.Thinking != nil {
				text = *block.Thinking
			}
			reasoning := types.ResponsesOutputReasoning{
				ID:      utils.MakeId("rs"),
				Type:    types.ResponsesItemTypeReasoning,
				Summary: []any{},
				Content: []types.ResponsesContentPart{
					{Type: "reasoning_text", Text: &text},
				},
				Status: new("completed"),
			}
			out = append(out, reasoning)
		}
	}

	if len(textChunks) > 0 {
		var fullText strings.Builder
		for _, chunk := range textChunks {
			fullText.WriteString(chunk)
		}
		message := types.ResponsesOutputMessage{
			ID:     utils.MakeId("msg"),
			Type:   types.ResponsesItemTypeMessage,
			Role:   "assistant",
			Status: "completed",
			Content: []map[string]any{
				{"type": "output_text", "text": fullText.String()},
			},
		}
		out = append(out, message)
	}

	return out
}
