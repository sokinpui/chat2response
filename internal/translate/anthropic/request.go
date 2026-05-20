package anthropic

import (
	"strings"

	"github.com/sokinpui/chat2response/internal/types"
	"github.com/sokinpui/chat2response/internal/utils"
)

var anthropicBuiltinToolTypes = map[string]bool{
	"web_search_20250305":   true,
	"computer_use_20250124": true,
	"text_editor_20250124":  true,
	"bash_20250124":         true,
}

var defaultReasoningBudgets = map[string]int{
	"minimal": 1024,
	"low":     4096,
	"medium":  16384,
	"high":    32768,
	"xhigh":   65536,
}

type TranslateRequestOptions struct {
	DefaultMaxTokens *int
	ReasoningBudgets map[string]int
}

type TranslateRequestResult struct {
	Request        types.AnthropicRequest
	HasPromptCache bool
}

func TranslateRequest(data types.ResponsesRequest, options TranslateRequestOptions) TranslateRequestResult {
	model := data.Model
	maxTokens := 8192
	if data.MaxOutputTokens != nil && *data.MaxOutputTokens > 0 {
		maxTokens = *data.MaxOutputTokens
	} else if data.MaxTokens != nil && *data.MaxTokens > 0 {
		maxTokens = *data.MaxTokens
	} else if options.DefaultMaxTokens != nil {
		maxTokens = *options.DefaultMaxTokens
	}

	systemBlocks := extractSystemBlocks(data.Instructions)
	built := buildMessages(data, systemBlocks)
	messages := built.Messages
	hasPromptCache := built.HasPromptCache

	if data.Extra != nil && data.Extra["prompt_cache_key"] != nil {
		hasPromptCache = true
		systemBlocks = markBlocksForCache(systemBlocks)
		messages = markCacheBreakpoint(messages)
	}

	messages = repairToolAdjacency(messages)
	messages = sanitizeMessages(messages)
	messages = ensureEndsWithUser(messages)

	if data.Extra != nil && data.Extra["prompt_cache_key"] != nil {
		messages = markCacheBreakpoint(messages)
	}

	request := types.AnthropicRequest{
		Model:     model,
		Messages:  messages,
		MaxTokens: maxTokens,
	}

	if len(systemBlocks) > 0 {
		request.System = systemBlocks
	}
	if data.Temperature != nil {
		request.Temperature = data.Temperature
	}
	if data.TopP != nil {
		request.TopP = data.TopP
	}

	tools := mapTools(data.Tools)
	if len(tools) > 0 {
		request.Tools = tools
		if choice := mapToolChoice(data.ToolChoice); choice != nil {
			request.ToolChoice = choice
		}
	}

	if data.Metadata != nil {
		request.Metadata = data.Metadata
	}

	thinking := mapThinking(data, maxTokens, options.ReasoningBudgets)
	if thinking != nil {
		request.Thinking = thinking
	}

	return TranslateRequestResult{
		Request:        request,
		HasPromptCache: hasPromptCache,
	}
}

func extractSystemBlocks(instructions interface{}) []types.AnthropicTextBlock {
	if instructions == nil {
		return nil
	}
	if s, ok := instructions.(string); ok {
		return []types.AnthropicTextBlock{{Type: "text", Text: s}}
	}

	arr, ok := instructions.([]interface{})
	if !ok {
		return nil
	}

	var blocks []types.AnthropicTextBlock
	for _, item := range arr {
		if s, ok := item.(string); ok {
			blocks = append(blocks, types.AnthropicTextBlock{Type: "text", Text: s})
			continue
		}
		if m, ok := item.(map[string]interface{}); ok {
			text, _ := m["text"].(string)
			block := types.AnthropicTextBlock{
				Type: "text",
				Text: text,
			}
			if cc, ok := m["cache_control"].(map[string]interface{}); ok {
				block.CacheControl = cc
			}
			blocks = append(blocks, block)
		}
	}
	return blocks
}

type buildResult struct {
	Messages       []types.AnthropicMessage
	HasPromptCache bool
}

func buildMessages(data types.ResponsesRequest, systemBlocks []types.AnthropicTextBlock) buildResult {
	messages := []types.AnthropicMessage{}
	hasPromptCache := false

	var pendingToolUses []types.AnthropicToolUseBlock
	var pendingToolResults []types.AnthropicToolResultBlock

	flushToolUses := func() {
		if len(pendingToolUses) == 0 {
			return
		}
		content := make([]types.AnthropicContentBlock, len(pendingToolUses))
		for i, use := range pendingToolUses {
			content[i] = types.AnthropicContentBlock{
				Type:         "tool_use",
				ID:           &use.ID,
				Name:         &use.Name,
				Input:        use.Input,
				CacheControl: use.CacheControl,
			}
		}
		messages = append(messages, types.AnthropicMessage{Role: "assistant", Content: content})
		pendingToolUses = nil
	}

	flushToolResults := func() {
		if len(pendingToolResults) == 0 {
			return
		}
		content := make([]types.AnthropicContentBlock, len(pendingToolResults))
		for i, res := range pendingToolResults {
			content[i] = types.AnthropicContentBlock{
				Type:         "tool_result",
				ToolUseID:    &res.ToolUseID,
				Content:      res.Content,
				CacheControl: res.CacheControl,
			}
		}
		messages = append(messages, types.AnthropicMessage{Role: "user", Content: content})
		pendingToolResults = nil
	}

	flushPending := func() {
		flushToolUses()
		flushToolResults()
	}

	inputItems := []interface{}{}
	if s, ok := data.Input.(string); ok {
		inputItems = append(inputItems, s)
	} else if arr, ok := data.Input.([]interface{}); ok {
		inputItems = arr
	}

	for _, raw := range inputItems {
		if s, ok := raw.(string); ok {
			flushPending()
			messages = append(messages, types.AnthropicMessage{
				Role:    "user",
				Content: []types.AnthropicContentBlock{{Type: "text", Text: &s}},
			})
			continue
		}

		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		itemType, _ := item["type"].(string)
		if itemType == "" {
			itemType = "message"
		}

		switch itemType {
		case "function_call_output", "commandExecutionOutput", "fileChangeOutput", "custom_tool_call_output":
			flushToolUses()
			callID, _ := item["call_id"].(string)
			if callID == "" {
				callID, _ = item["id"].(string)
			}
			if callID == "" {
				callID = utils.MakeId("call")
			}
			pendingToolResults = append(pendingToolResults, types.AnthropicToolResultBlock{
				Type:      "tool_result",
				ToolUseID: callID,
				Content:   extractToolOutputText(item),
			})
			continue

		case "function_call", "commandExecution", "local_shell_call", "fileChange", "custom_tool_call", "web_search_call":
			flushToolResults()
			if block := mapInputToolCall(item); block != nil {
				pendingToolUses = append(pendingToolUses, *block)
			}
			continue

		case "reasoning":
			flushPending()
			continue

		case "message", "agentMessage":
			flushPending()
			role, _ := item["role"].(string)
			if role == "" {
				role = "user"
			}
			if role == "developer" {
				role = "system"
			}

			if role == "system" {
				if text := extractMessageText(item); text != "" {
					systemBlocks = append(systemBlocks, types.AnthropicTextBlock{Type: "text", Text: text})
				}
				continue
			}

			var contentBlocks []types.AnthropicContentBlock
			rawContent := item["content"]

			if s, ok := rawContent.(string); ok {
				contentBlocks = append(contentBlocks, types.AnthropicContentBlock{Type: "text", Text: &s})
			} else if arr, ok := rawContent.([]interface{}); ok {
				for _, part := range arr {
					if s, ok := part.(string); ok {
						contentBlocks = append(contentBlocks, types.AnthropicContentBlock{Type: "text", Text: &s})
						continue
					}
					m, ok := part.(map[string]interface{})
					if !ok {
						continue
					}
					pType, _ := m["type"].(string)
					if pType == "input_text" || pType == "text" || pType == "output_text" {
						text, _ := m["text"].(string)
						block := types.AnthropicContentBlock{Type: "text", Text: &text}
						if cc, ok := m["cache_control"].(map[string]interface{}); ok {
							block.CacheControl = cc
						}
						contentBlocks = append(contentBlocks, block)
					} else if pType == "input_image" || pType == "image" || pType == "image_url" {
						url := extractImageUrl(m)
						if strings.HasPrefix(url, "data:") {
							if mediaType, base64Data, ok := parseDataUrl(url); ok {
								contentBlocks = append(contentBlocks, types.AnthropicContentBlock{
									Type: "image",
									Source: &types.AnthropicImageSource{
										Type:      "base64",
										MediaType: &mediaType,
										Data:      &base64Data,
									},
								})
							}
						} else if url != "" {
							contentBlocks = append(contentBlocks, types.AnthropicContentBlock{
								Type: "image",
								Source: &types.AnthropicImageSource{
									Type: "url",
									URL:  &url,
								},
							})
						}
					} else if pType == "input_file" {
						data, _ := m["data"].(string)
						mime, _ := m["mime_type"].(string)
						if mime == "" {
							mime = "application/pdf"
						}
						contentBlocks = append(contentBlocks, types.AnthropicContentBlock{
							Type: "document",
							Source: &types.AnthropicImageSource{
								Type:      "base64",
								MediaType: &mime,
								Data:      &data,
							},
						})
					}
				}
			}

			if role == "assistant" || role == "model" {
				if len(contentBlocks) > 0 {
					messages = append(messages, types.AnthropicMessage{Role: "assistant", Content: contentBlocks})
				}
			} else {
				if len(contentBlocks) > 0 {
					messages = append(messages, types.AnthropicMessage{Role: "user", Content: contentBlocks})
				}
			}
		}
	}

	flushPending()

	for _, block := range systemBlocks {
		if block.CacheControl != nil {
			hasPromptCache = true
			break
		}
	}

	return buildResult{Messages: messages, HasPromptCache: hasPromptCache}
}

func extractMessageText(item map[string]interface{}) string {
	rawContent := item["content"]
	if s, ok := rawContent.(string); ok {
		return s
	}
	if arr, ok := rawContent.([]interface{}); ok {
		var sb strings.Builder
		for _, part := range arr {
			if s, ok := part.(string); ok {
				sb.WriteString(s)
			} else if m, ok := part.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok {
					sb.WriteString(text)
				}
			}
		}
		return sb.String()
	}
	return ""
}

func extractToolOutputText(item map[string]interface{}) string {
	raw := item["output"]
	if raw == nil {
		raw = item["content"]
	}
	if raw == nil {
		raw = item["stdout"]
	}
	if raw == nil {
		return ""
	}

	if s, ok := raw.(string); ok {
		return s
	}
	if arr, ok := raw.([]interface{}); ok {
		var sb strings.Builder
		for _, part := range arr {
			if s, ok := part.(string); ok {
				sb.WriteString(s)
			} else if m, ok := part.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok {
					sb.WriteString(text)
				}
			}
		}
		return sb.String()
	}

	if m, ok := raw.(map[string]interface{}); ok {
		if content, ok := m["content"].(string); ok {
			return content
		}
	}
	return ""
}

func mapInputToolCall(item map[string]interface{}) *types.AnthropicToolUseBlock {
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
		return nil
	}

	input := make(map[string]interface{})
	if args, ok := item["arguments"].(map[string]interface{}); ok {
		input = args
	} else if in, ok := item["input"].(map[string]interface{}); ok {
		input = in
	}

	block := &types.AnthropicToolUseBlock{
		Type:  "tool_use",
		ID:    callID,
		Name:  name,
		Input: input,
	}

	if cc, ok := item["cache_control"].(map[string]interface{}); ok {
		block.CacheControl = cc
	}

	return block
}

func extractImageUrl(m map[string]interface{}) string {
	if imgUrl, ok := m["image_url"].(string); ok {
		return imgUrl
	}
	if imgObj, ok := m["image_url"].(map[string]interface{}); ok {
		if url, ok := imgObj["url"].(string); ok {
			return url
		}
	}
	return ""
}

func parseDataUrl(url string) (mediaType string, data string, ok bool) {
	if !strings.HasPrefix(url, "data:") {
		return "", "", false
	}
	parts := strings.SplitN(url[5:], ";base64,", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func mapTools(tools []types.ResponsesTool) []interface{} {
	var out []interface{}
	for _, tool := range tools {
		if anthropicBuiltinToolTypes[tool.Type] {
			out = append(out, tool)
			continue
		}
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

		params := map[string]interface{}{"type": "object"}
		if tool.Function != nil && tool.Function.Parameters != nil {
			params = tool.Function.Parameters
		} else if tool.Parameters != nil {
			params = tool.Parameters
		}

		desc := ""
		if tool.Function != nil && tool.Function.Description != nil {
			desc = *tool.Function.Description
		} else if tool.Description != nil {
			desc = *tool.Description
		}

		out = append(out, types.AnthropicTool{
			Name:        name,
			Description: &desc,
			InputSchema: params,
		})
	}
	return out
}

func mapToolChoice(choice interface{}) *types.AnthropicToolChoice {
	if choice == nil || choice == "auto" {
		return &types.AnthropicToolChoice{Type: "auto"}
	}
	if choice == "required" {
		return &types.AnthropicToolChoice{Type: "any"}
	}
	if choice == "none" {
		return nil
	}
	if m, ok := choice.(map[string]interface{}); ok {
		cType, _ := m["type"].(string)
		if cType == "function" {
			if fn, ok := m["function"].(map[string]interface{}); ok {
				if name, ok := fn["name"].(string); ok {
					return &types.AnthropicToolChoice{Type: "tool", Name: name}
				}
			}
		}
		if cType == "auto" || cType == "any" {
			return &types.AnthropicToolChoice{Type: cType}
		}
	}
	return &types.AnthropicToolChoice{Type: "auto"}
}

func mapThinking(data types.ResponsesRequest, maxTokens int, overrides map[string]int) *types.AnthropicThinkingConfig {
	if data.Reasoning == nil || data.Reasoning.Effort == nil {
		return nil
	}
	effort := *data.Reasoning.Effort
	if effort == "minimal" {
		return nil
	}

	budgets := make(map[string]int)
	for k, v := range defaultReasoningBudgets {
		budgets[k] = v
	}
	for k, v := range overrides {
		budgets[k] = v
	}

	budget, ok := budgets[effort]
	if !ok {
		budget = defaultReasoningBudgets["medium"]
	}

	clamped := budget
	if maxTokens-1024 < clamped {
		clamped = maxTokens - 1024
	}
	if clamped < 1024 {
		clamped = 1024
	}

	return &types.AnthropicThinkingConfig{
		Type:         "enabled",
		BudgetTokens: &clamped,
	}
}

func repairToolAdjacency(messages []types.AnthropicMessage) []types.AnthropicMessage {
	repaired := []types.AnthropicMessage{}
	working := make([]types.AnthropicMessage, len(messages))
	copy(working, messages)

	for i := 0; i < len(working); i++ {
		msg := working[i]
		repaired = append(repaired, msg)
		if msg.Role != "assistant" {
			continue
		}

		content, ok := msg.Content.([]types.AnthropicContentBlock)
		if !ok {
			continue
		}

		var toolUseIDs []string
		for _, block := range content {
			if block.Type == "tool_use" && block.ID != nil {
				toolUseIDs = append(toolUseIDs, *block.ID)
			}
		}
		if len(toolUseIDs) == 0 {
			continue
		}

		var next *types.AnthropicMessage
		if i+1 < len(working) {
			next = &working[i+1]
		}

		var nextUserContent []types.AnthropicContentBlock
		if next != nil && next.Role == "user" {
			if arr, ok := next.Content.([]types.AnthropicContentBlock); ok {
				nextUserContent = arr
			}
		}

		foundByID := make(map[string]types.AnthropicContentBlock)
		consumedInNext := make(map[string]bool)

		for _, block := range nextUserContent {
			if block.Type == "tool_result" && block.ToolUseID != nil {
				tid := *block.ToolUseID
				if contains(toolUseIDs, tid) && foundByID[tid].ToolUseID == nil {
					foundByID[tid] = block
					consumedInNext[tid] = true
				}
			}
		}

		missing := []string{}
		for _, id := range toolUseIDs {
			if _, ok := foundByID[id]; !ok {
				missing = append(missing, id)
			}
		}

		if len(missing) > 0 {
			missingSet := make(map[string]bool)
			for _, id := range missing {
				missingSet[id] = true
			}

			for j := i + 2; j < len(working) && len(missingSet) > 0; j++ {
				later := &working[j]
				if later.Role != "user" {
					continue
				}
				arr, ok := later.Content.([]types.AnthropicContentBlock)
				if !ok {
					continue
				}

				var keep []types.AnthropicContentBlock
				for _, block := range arr {
					if block.Type == "tool_result" && block.ToolUseID != nil {
						tid := *block.ToolUseID
						if missingSet[tid] {
							foundByID[tid] = block
							delete(missingSet, tid)
							continue
						}
					}
					keep = append(keep, block)
				}
				later.Content = keep
			}
		}

		ordered := make([]types.AnthropicContentBlock, len(toolUseIDs))
		for idx, id := range toolUseIDs {
			if block, ok := foundByID[id]; ok {
				ordered[idx] = block
			} else {
				empty := ""
				ordered[idx] = types.AnthropicContentBlock{
					Type:      "tool_result",
					ToolUseID: &id,
					Content:   empty,
				}
			}
		}

		repaired = append(repaired, types.AnthropicMessage{Role: "user", Content: ordered})

		if len(nextUserContent) > 0 {
			var remaining []types.AnthropicContentBlock
			for _, block := range nextUserContent {
				if block.Type == "tool_result" && block.ToolUseID != nil {
					if consumedInNext[*block.ToolUseID] {
						delete(consumedInNext, *block.ToolUseID)
						continue
					}
				}
				remaining = append(remaining, block)
			}
			if len(remaining) > 0 {
				repaired = append(repaired, types.AnthropicMessage{Role: "user", Content: remaining})
			}
			i++
		}
	}

	return repaired
}

func sanitizeMessages(messages []types.AnthropicMessage) []types.AnthropicMessage {
	var out []types.AnthropicMessage
	for _, msg := range messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		if arr, ok := msg.Content.([]types.AnthropicContentBlock); ok {
			var blocks []types.AnthropicContentBlock
			for _, block := range arr {
				if block.Type == "text" && (block.Text == nil || *block.Text == "") {
					continue
				}
				blocks = append(blocks, block)
			}
			if len(blocks) > 0 {
				out = append(out, types.AnthropicMessage{Role: msg.Role, Content: blocks})
			}
		} else if s, ok := msg.Content.(string); ok && s != "" {
			out = append(out, types.AnthropicMessage{
				Role:    msg.Role,
				Content: []types.AnthropicContentBlock{{Type: "text", Text: &s}},
			})
		}
	}
	return out
}

func ensureEndsWithUser(messages []types.AnthropicMessage) []types.AnthropicMessage {
	if len(messages) == 0 {
		text := "..."
		return []types.AnthropicMessage{{
			Role:    "user",
			Content: []types.AnthropicContentBlock{{Type: "text", Text: &text}},
		}}
	}
	last := messages[len(messages)-1]
	if last.Role == "user" {
		return messages
	}
	text := "Continue."
	return append(messages, types.AnthropicMessage{
		Role:    "user",
		Content: []types.AnthropicContentBlock{{Type: "text", Text: &text}},
	})
}

func markBlocksForCache(blocks []types.AnthropicTextBlock) []types.AnthropicTextBlock {
	count := 0
	for i := range blocks {
		if blocks[i].CacheControl == nil {
			blocks[i].CacheControl = map[string]interface{}{"type": "ephemeral"}
			count++
			if count >= 3 {
				break
			}
		}
	}
	return blocks
}

func markCacheBreakpoint(messages []types.AnthropicMessage) []types.AnthropicMessage {
	for i := range messages {
		if messages[i].Role == "assistant" {
			if arr, ok := messages[i].Content.([]types.AnthropicContentBlock); ok {
				for j := len(arr) - 1; j >= 0; j-- {
					if arr[j].Type == "text" && arr[j].CacheControl == nil {
						arr[j].CacheControl = map[string]interface{}{"type": "ephemeral"}
						return messages
					}
				}
			}
		}
	}

	for i := range messages {
		if messages[i].Role == "user" {
			if arr, ok := messages[i].Content.([]types.AnthropicContentBlock); ok {
				for j := len(arr) - 1; j >= 0; j-- {
					if arr[j].Type == "text" && arr[j].CacheControl == nil {
						arr[j].CacheControl = map[string]interface{}{"type": "ephemeral"}
						return messages
					}
				}
			}
		}
	}

	return messages
}

func contains(arr []string, s string) bool {
	for _, item := range arr {
		if item == s {
			return true
		}
	}
	return false
}

func ptr[T any](v T) *T {
	return &v
}
