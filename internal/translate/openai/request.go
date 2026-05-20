package openai

import (
	"fmt"
	"strings"

	"github.com/sokinpui/chat2response/internal/types"
	"github.com/sokinpui/chat2response/internal/utils"
)

type TranslateRequestOptions struct {
	DefaultMaxTokens     *int
	BackfillReasoning    *bool
	ReasoningPlaceholder *string
	DropImages           bool
}

type TranslateRequestResult struct {
	Request types.OpenAiChatRequest
}

func TranslateRequest(data types.ResponsesRequest, options TranslateRequestOptions) TranslateRequestResult {
	messages := []types.OpenAiChatMessage{}

	if systemContent := buildSystemContent(data.Instructions); systemContent != "" {
		messages = append(messages, types.OpenAiChatMessage{
			Role:    types.OpenAiRoleSystem,
			Content: systemContent,
		})
	}

	inputItems := []any{}
	if s, ok := data.Input.(string); ok {
		inputItems = append(inputItems, s)
	} else if arr, ok := data.Input.([]any); ok {
		inputItems = arr
	}

	for _, raw := range inputItems {
		if s, ok := raw.(string); ok {
			messages = append(messages, types.OpenAiChatMessage{
				Role:    types.OpenAiRoleUser,
				Content: s,
			})
			continue
		}

		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		messages = processInputItem(item, messages, options.DropImages)
	}

	request := types.OpenAiChatRequest{
		Model:    data.Model,
		Messages: messages,
	}

	if data.Temperature != nil {
		request.Temperature = data.Temperature
	}
	if data.TopP != nil {
		request.TopP = data.TopP
	}

	if data.Reasoning != nil && data.Reasoning.Effort != nil {
		if request.Extra == nil {
			request.Extra = make(map[string]any)
		}
		request.Extra["reasoning_effort"] = *data.Reasoning.Effort
	}

	maxTokens := options.DefaultMaxTokens
	if data.MaxOutputTokens != nil && *data.MaxOutputTokens > 0 {
		maxTokens = data.MaxOutputTokens
	} else if data.MaxTokens != nil && *data.MaxTokens > 0 {
		maxTokens = data.MaxTokens
	}
	request.MaxTokens = maxTokens

	tools := mapTools(data.Tools)
	if len(tools) > 0 {
		request.Tools = tools
		if choice := mapToolChoice(data.ToolChoice); choice != nil {
			request.ToolChoice = choice
		}
	}

	backfillReasoning(messages, options)
	request.Messages = repairToolMessageOrder(messages)

	return TranslateRequestResult{Request: request}
}

func buildSystemContent(instructions any) string {
	if instructions == nil {
		return ""
	}
	if s, ok := instructions.(string); ok {
		return s
	}
	arr, ok := instructions.([]any)
	if !ok {
		return ""
	}

	var sb strings.Builder
	for _, block := range arr {
		if s, ok := block.(string); ok {
			sb.WriteString(s)
			continue
		}
		if m, ok := block.(map[string]any); ok {
			if text, ok := m["text"].(string); ok {
				sb.WriteString(text)
			}
		}
	}
	return sb.String()
}

func processInputItem(item map[string]any, messages []types.OpenAiChatMessage, dropImages bool) []types.OpenAiChatMessage {
	itemType, _ := item["type"].(string)
	if itemType == "" {
		itemType = "message"
	}

	getLastAssistantIdx := func() int {
		if len(messages) > 0 && messages[len(messages)-1].Role == types.OpenAiRoleAssistant {
			return len(messages) - 1
		}
		messages = append(messages, types.OpenAiChatMessage{Role: types.OpenAiRoleAssistant, Content: nil})
		return len(messages) - 1
	}

	if itemType == "message" || itemType == "agentMessage" {
		role, _ := item["role"].(string)
		if role == "" {
			role = "user"
		}
		if role == "developer" {
			role = "system"
		}

		reasoningContent := ""
		if rc, ok := item["reasoning_content"].(string); ok {
			reasoningContent = rc
		}
		rawContent := item["content"]

		if role == "assistant" || role == "model" {
			content := ""
			if s, ok := rawContent.(string); ok {
				content = s
			} else if arr, ok := rawContent.([]any); ok {
				for _, part := range arr {
					if s, ok := part.(string); ok {
						content += s
						continue
					}
					if m, ok := part.(map[string]any); ok {
						pType, _ := m["type"].(string)
						pText, _ := m["text"].(string)
						if pType == "input_text" || pType == "text" || pType == "output_text" {
							content += pText
						} else if pType == "reasoning_text" {
							reasoningContent += pText
						}
					}
				}
			}
			idx := getLastAssistantIdx()
			if content != "" {
				current, _ := messages[idx].Content.(string)
				messages[idx].Content = current + content
			}
			if reasoningContent != "" {
				current := ""
				if messages[idx].ReasoningContent != nil {
					current = *messages[idx].ReasoningContent
				}
				combined := current + reasoningContent
				messages[idx].ReasoningContent = &combined
			}
			if sig, ok := item["thought_signature"].(string); ok && sig != "" {
				if messages[idx].Extra == nil {
					messages[idx].Extra = make(map[string]any)
				}
				messages[idx].Extra["thought_signature"] = sig
			}
		} else {
			if s, ok := rawContent.(string); ok {
				messages = append(messages, types.OpenAiChatMessage{Role: role, Content: s})
			} else if arr, ok := rawContent.([]any); ok {
				contentBlocks := []map[string]any{}
				for _, part := range arr {
					if s, ok := part.(string); ok {
						contentBlocks = append(contentBlocks, map[string]any{"type": "text", "text": s})
						continue
					}
					m, ok := part.(map[string]any)
					if !ok {
						continue
					}
					pType, _ := m["type"].(string)
					if pType == "input_text" || pType == "text" || pType == "output_text" {
						pText, _ := m["text"].(string)
						contentBlocks = append(contentBlocks, map[string]any{"type": "text", "text": pText})
					} else if pType == "reasoning_text" {
						pText, _ := m["text"].(string)
						reasoningContent += pText
					} else if pType == "input_image" || pType == "image" || pType == "image_url" {
						if dropImages {
							continue
						}
						url := extractImageUrl(m)
						if url != "" {
							contentBlocks = append(contentBlocks, map[string]any{
								"type":      "image_url",
								"image_url": map[string]any{"url": url},
							})
						}
					} else if pType == "input_file" || pType == "file" {
						url := extractFileUrl(m)
						if url != "" {
							contentBlocks = append(contentBlocks, map[string]any{
								"type":      "image_url",
								"image_url": map[string]any{"url": url},
							})
						}
					}
				}
				msg := types.OpenAiChatMessage{Role: role, Content: contentBlocks}
				if reasoningContent != "" {
					msg.ReasoningContent = &reasoningContent
				}
				if sig, ok := item["thought_signature"].(string); ok && sig != "" {
					if msg.Extra == nil {
						msg.Extra = make(map[string]any)
					}
					msg.Extra["thought_signature"] = sig
				}
				messages = append(messages, msg)
			} else {
				messages = append(messages, types.OpenAiChatMessage{Role: role, Content: ""})
			}
		}
		return messages
	}

	if itemType == "reasoning" {
		content := ""
		raw := item["content"]
		if arr, ok := raw.([]any); ok {
			for _, cp := range arr {
				if s, ok := cp.(string); ok {
					content += s
				} else if m, ok := cp.(map[string]any); ok {
					if text, ok := m["text"].(string); ok {
						content += text
					}
				}
			}
		} else if s, ok := raw.(string); ok {
			content = s
		}
		idx := getLastAssistantIdx()
		current := ""
		if messages[idx].ReasoningContent != nil {
			current = *messages[idx].ReasoningContent
		}
		combined := current + content
		messages[idx].ReasoningContent = &combined
		if sig, ok := item["thought_signature"].(string); ok && sig != "" {
			if messages[idx].Extra == nil {
				messages[idx].Extra = make(map[string]any)
			}
			messages[idx].Extra["thought_signature"] = sig
		}
		return messages
	}

	switch itemType {
	case "function_call", "commandExecution", "local_shell_call", "fileChange", "custom_tool_call", "web_search_call":
		return processToolCall(item, messages, getLastAssistantIdx)
	case "function_call_output", "commandExecutionOutput", "fileChangeOutput", "custom_tool_call_output":
		return processToolOutput(item, messages)
	}

	return messages
}

func extractImageUrl(m map[string]any) string {
	if imgUrl, ok := m["image_url"].(string); ok {
		return imgUrl
	}
	if imgObj, ok := m["image_url"].(map[string]any); ok {
		if url, ok := imgObj["url"].(string); ok {
			return url
		}
	}
	data, _ := m["data"].(string)
	if data == "" {
		data, _ = m["base64"].(string)
	}
	if data == "" {
		return ""
	}
	mime, _ := m["mime_type"].(string)
	if mime == "" {
		mime, _ = m["media_type"].(string)
	}
	if mime == "" {
		mime = "image/png"
	}
	if strings.HasPrefix(data, "data:") {
		return data
	}
	return fmt.Sprintf("data:%s;base64,%s", mime, data)
}

func extractFileUrl(m map[string]any) string {
	data, _ := m["file_data"].(string)
	if data == "" {
		data, _ = m["data"].(string)
	}
	if data == "" {
		return ""
	}
	mime, _ := m["mime_type"].(string)
	if mime == "" {
		mime, _ = m["media_type"].(string)
	}
	if mime == "" {
		mime = "application/pdf"
	}
	if strings.HasPrefix(data, "data:") {
		return data
	}
	return fmt.Sprintf("data:%s;base64,%s", mime, data)
}

func processToolCall(item map[string]any, messages []types.OpenAiChatMessage, getAssistantIdx func() int) []types.OpenAiChatMessage {
	callID, _ := item["call_id"].(string)
	if callID == "" {
		callID, _ = item["id"].(string)
	}
	if callID == "" {
		callID = utils.MakeId("call")
	}

	name, _ := item["name"].(string)
	itemType, _ := item["type"].(string)

	if name == "" {
		switch itemType {
		case "commandExecution":
			name = "run_shell_command"
		case "local_shell_call":
			name = "local_shell_command"
		case "fileChange":
			name = "write_file"
		case "web_search_call":
			name = "web_search"
		}
	}

	if name == "" {
		return messages
	}

	var args any
	if v, ok := item["arguments"]; ok {
		args = v
	} else if v, ok := item["input"]; ok {
		args = v
	}

	if isEmpty(args) {
		switch itemType {
		case "commandExecution":
			cmd, _ := item["command"].(string)
			cwd, _ := item["cwd"].(string)
			if cwd == "" {
				cwd = "."
			}
			args = map[string]string{"command": cmd, "dir_path": cwd}
		case "local_shell_call":
			if action, ok := item["action"].(map[string]any); ok {
				if exec, ok := action["exec"].(map[string]any); ok {
					args = map[string]any{
						"command":           exec["command"],
						"working_directory": exec["working_directory"],
					}
				}
			}
		case "fileChange":
			if changes, ok := item["changes"].([]any); ok && len(changes) > 0 {
				if first, ok := changes[0].(map[string]any); ok {
					args = map[string]any{"file_path": first["path"]}
				}
			}
		case "web_search_call":
			if action, ok := item["action"]; ok {
				args = action
			}
		}
	}

	if args == nil {
		args = make(map[string]any)
	}

	argsStr := ""
	if s, ok := args.(string); ok {
		argsStr = s
	} else {
		argsStr = utils.JsonStringifySafe(args)
	}

	idx := getAssistantIdx()
	messages[idx].ToolCalls = append(messages[idx].ToolCalls, types.OpenAiChatToolCall{
		ID:   &callID,
		Type: new("function"),
		Function: &struct {
			Name      *string `json:"name,omitempty"`
			Arguments any     `json:"arguments,omitempty"`
		}{
			Name:      &name,
			Arguments: argsStr,
		},
	})

	if sig, ok := item["thought_signature"].(string); ok && sig != "" {
		if messages[idx].Extra == nil {
			messages[idx].Extra = make(map[string]any)
		}
		messages[idx].Extra["thought_signature"] = sig
	}
	if thought, ok := item["thought"].(string); ok && thought != "" {
		current := ""
		if messages[idx].ReasoningContent != nil {
			current = *messages[idx].ReasoningContent
		}
		combined := current + thought
		messages[idx].ReasoningContent = &combined
	}

	return messages
}

func processToolOutput(item map[string]any, messages []types.OpenAiChatMessage) []types.OpenAiChatMessage {
	callID, _ := item["call_id"].(string)
	outputRaw := item["output"]
	if outputRaw == nil {
		outputRaw = item["content"]
	}
	if outputRaw == nil {
		outputRaw = item["stdout"]
	}

	content := ""
	if s, ok := outputRaw.(string); ok {
		content = s
	} else if arr, ok := outputRaw.([]any); ok {
		for _, part := range arr {
			if s, ok := part.(string); ok {
				content += s
			} else if m, ok := part.(map[string]any); ok {
				pType, _ := m["type"].(string)
				if pType == "input_text" || pType == "text" {
					if text, ok := m["text"].(string); ok {
						content += text
					}
				}
			}
		}
	} else if obj, ok := outputRaw.(map[string]any); ok {
		content, _ = obj["content"].(string)
		if content == "" && obj["success"] == false {
			content = "Error: Tool execution failed"
		}
	}

	if content == "" {
		if stderr, ok := item["stderr"].(string); ok && stderr != "" {
			content = "Error: " + stderr
		}
	}

	messages = append(messages, types.OpenAiChatMessage{
		Role:       types.OpenAiRoleTool,
		ToolCallID: &callID,
		Content:    content,
	})
	return messages
}

func mapTools(tools []types.ResponsesTool) []any {
	out := []any{}
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		name := ""
		if tool.Function != nil {
			name = tool.Function.Name
		} else if tool.Name != nil {
			name = *tool.Name
		}
		if name == "" {
			continue
		}

		params := map[string]any{"type": "object"}
		if tool.Function != nil && tool.Function.Parameters != nil {
			params = tool.Function.Parameters
		} else if tool.Parameters != nil {
			params = tool.Parameters
		}

		description := ""
		if tool.Function != nil && tool.Function.Description != nil {
			description = *tool.Function.Description
		} else if tool.Description != nil {
			description = *tool.Description
		}

		out = append(out, types.OpenAiChatFunctionTool{
			Type: "function",
			Function: struct {
				Name        string         `json:"name"`
				Description *string        `json:"description,omitempty"`
				Parameters  map[string]any `json:"parameters,omitempty"`
			}{
				Name:        name,
				Description: &description,
				Parameters:  params,
			},
		})
	}
	return out
}

func mapToolChoice(choice any) any {
	if choice == nil {
		return nil
	}
	if s, ok := choice.(string); ok {
		if s == "auto" || s == "required" || s == "none" {
			return s
		}
	}
	if m, ok := choice.(map[string]any); ok {
		if m["type"] == "function" {
			if fn, ok := m["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok {
					return map[string]any{
						"type":     "function",
						"function": map[string]string{"name": name},
					}
				}
			}
		}
		return choice
	}
	return nil
}

func backfillReasoning(messages []types.OpenAiChatMessage, options TranslateRequestOptions) {
	if options.BackfillReasoning != nil && !*options.BackfillReasoning {
		return
	}
	placeholder := "."
	if options.ReasoningPlaceholder != nil {
		placeholder = *options.ReasoningPlaceholder
	}
	if placeholder == "" {
		return
	}

	for i := range messages {
		if messages[i].Role == types.OpenAiRoleAssistant && messages[i].ReasoningContent == nil {
			messages[i].ReasoningContent = &placeholder
		}
	}
}

func repairToolMessageOrder(messages []types.OpenAiChatMessage) []types.OpenAiChatMessage {
	if len(messages) == 0 {
		return messages
	}

	type Block struct {
		Assistant types.OpenAiChatMessage
		Trailing  []types.OpenAiChatMessage
	}

	var blocks []Block
	var currentBlock *Block

	for _, msg := range messages {
		if msg.Role == types.OpenAiRoleAssistant {
			blocks = append(blocks, Block{Assistant: msg})
			currentBlock = &blocks[len(blocks)-1]
		} else if currentBlock != nil {
			currentBlock.Trailing = append(currentBlock.Trailing, msg)
		} else {
			blocks = append(blocks, Block{
				Assistant: types.OpenAiChatMessage{Role: types.OpenAiRoleAssistant, Content: nil},
				Trailing:  []types.OpenAiChatMessage{msg},
			})
		}
	}

	var result []types.OpenAiChatMessage
	for _, block := range blocks {
		toolCallIDs := make(map[string]bool)
		for _, tc := range block.Assistant.ToolCalls {
			if tc.ID != nil {
				toolCallIDs[*tc.ID] = true
			}
		}

		if len(toolCallIDs) == 0 {
			if block.Assistant.ToolCalls != nil || block.Assistant.Content != nil {
				result = append(result, block.Assistant)
			}
			result = append(result, block.Trailing...)
			continue
		}

		var tools []types.OpenAiChatMessage
		var others []types.OpenAiChatMessage
		for _, m := range block.Trailing {
			if m.Role == types.OpenAiRoleTool && m.ToolCallID != nil && toolCallIDs[*m.ToolCallID] {
				tools = append(tools, m)
			} else {
				others = append(others, m)
			}
		}

		if block.Assistant.ToolCalls != nil || block.Assistant.Content != nil {
			result = append(result, block.Assistant)
		}
		result = append(result, tools...)
		result = append(result, others...)
	}

	return result
}

func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case string:
		return len(v) == 0
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}

//go:fix inline
func ptr[T any](v T) *T {
	return new(v)
}
