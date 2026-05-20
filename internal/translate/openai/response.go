package openai

import (
	"time"

	"github.com/sokinpui/chat2response/internal/types"
	"github.com/sokinpui/chat2response/internal/utils"
)

var SHELL_TOOL_NAMES = map[string]bool{
	"shell":          true,
	"container.exec": true,
	"shell_command":  true,
}

type TranslateResponseOptions struct {
	ResponseID *string
	CreatedAt  *int64
	Model      *string
}

func TranslateResponse(body types.OpenAiChatResponse, options TranslateResponseOptions) types.ResponsesResponse {
	createdAt := time.Now().Unix()
	if body.Created != nil {
		createdAt = *body.Created
	}
	if options.CreatedAt != nil {
		createdAt = *options.CreatedAt
	}

	id := utils.MakeId("resp")
	if body.ID != nil {
		id = *body.ID
	}
	if options.ResponseID != nil {
		id = *options.ResponseID
	}

	model := ""
	if body.Model != nil {
		model = *body.Model
	}
	if options.Model != nil {
		model = *options.Model
	}

	var output []interface{}
	if len(body.Choices) > 0 {
		msg := body.Choices[0].Message
		for _, tc := range msg.ToolCalls {
			if item := mapToolCallToOutput(tc); item != nil {
				output = append(output, item)
			}
		}

		if msg.Content != nil && *msg.Content != "" {
			output = append(output, types.ResponsesOutputMessage{
				ID:     utils.MakeId("msg"),
				Type:   types.ResponsesItemTypeMessage,
				Role:   types.OpenAiRoleAssistant,
				Status: "completed",
				Content: []map[string]interface{}{
					{"type": "output_text", "text": *msg.Content},
				},
			})
		}
	}

	usage := types.ResponsesUsage{}
	if body.Usage != nil {
		in := 0
		if body.Usage.PromptTokens != nil {
			in = *body.Usage.PromptTokens
		}
		out := 0
		if body.Usage.CompletionTokens != nil {
			out = *body.Usage.CompletionTokens
		}
		total := in + out
		if body.Usage.TotalTokens != nil {
			total = *body.Usage.TotalTokens
		}
		usage.InputTokens = in
		usage.OutputTokens = out
		usage.TotalTokens = total

		if body.Usage.PromptTokensDetails != nil {
			usage.InputTokensDetails = &struct {
				CachedTokens        *int `json:"cached_tokens,omitempty"`
				CacheCreationTokens *int `json:"cache_creation_tokens,omitempty"`
			}{
				CachedTokens: body.Usage.PromptTokensDetails.CachedTokens,
			}
		}
	}

	return types.ResponsesResponse{
		ID:        id,
		Object:    "response",
		CreatedAt: createdAt,
		Model:     model,
		Status:    "completed",
		Output:    output,
		Usage:     &usage,
	}
}

func mapToolCallToOutput(tc types.OpenAiChatToolCall) *types.ResponsesOutputFunctionCall {
	if tc.Function == nil || tc.Function.Name == nil {
		return nil
	}
	name := *tc.Function.Name
	callID := utils.MakeId("call")
	if tc.ID != nil {
		callID = *tc.ID
	}

	args := ""
	if s, ok := tc.Function.Arguments.(string); ok {
		args = s
	} else {
		args = utils.JsonStringifySafe(tc.Function.Arguments)
	}

	item := &types.ResponsesOutputFunctionCall{
		ID:        callID,
		Type:      types.ResponsesItemTypeFunctionCall,
		Status:    "completed",
		Name:      &name,
		Arguments: &args,
		CallID:    &callID,
	}

	if SHELL_TOOL_NAMES[name] {
		item.Type = types.ResponsesItemTypeLocalShellCall
		var parsed struct {
			Command []string `json:"command"`
		}
		_ = utils.SafeJsonParse(args, &parsed)
		item.Action = &struct {
			Type    *string  `json:"type,omitempty"`
			Command []string `json:"command,omitempty"`
		}{
			Type:    ptr("exec"),
			Command: parsed.Command,
		}
	}

	return item
}
