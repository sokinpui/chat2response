package openai

import (
	"sort"
	"time"

	"github.com/sokinpui/chat2response/internal/types"
	"github.com/sokinpui/chat2response/internal/utils"
)

type ResponsesStreamMetadata struct {
	Temperature *float64
	TopP        *float64
	Tools       []any
	ToolChoice  any
	Store       *bool
	Metadata    map[string]any
}

type TranslateStreamOptions struct {
	Model           *string
	ResponseID      *string
	CreatedAt       *int64
	RequestMetadata *ResponsesStreamMetadata
}

func TranslateStream(stream <-chan utils.SseMessage, options TranslateStreamOptions) <-chan types.ResponsesStreamEvent {
	out := make(chan types.ResponsesStreamEvent)
	go func() {
		defer close(out)
		translator := newStreamTranslator(options)
		out <- translator.createInitialEvent()

		for msg := range stream {
			if msg.Data == "[DONE]" {
				break
			}
			var chunk types.OpenAiChatStreamChunk
			if err := utils.SafeJsonParse(msg.Data, &chunk); err != nil {
				continue
			}
			for _, ev := range translator.handleChunk(chunk) {
				out <- ev
			}
		}

		for _, ev := range translator.finalize() {
			out <- ev
		}
	}()
	return out
}

type toolCallState struct {
	outputIndex int
	item        types.ResponsesOutputFunctionCall
}

type streamTranslator struct {
	model      string
	responseID string
	createdAt  int64
	metadata   ResponsesStreamMetadata
	seq        int
	outCounter int

	textItem      *types.ResponsesOutputMessage
	textItemIndex int
	textBuffer    string

	toolCalls map[int]*toolCallState

	inputTokens  int
	outputTokens int
	cachedTokens int
}

func newStreamTranslator(opts TranslateStreamOptions) *streamTranslator {
	st := &streamTranslator{
		model:         "",
		responseID:    utils.MakeId("resp"),
		createdAt:     time.Now().Unix(),
		textItemIndex: -1,
		toolCalls:     make(map[int]*toolCallState),
	}
	if opts.Model != nil {
		st.model = *opts.Model
	}
	if opts.ResponseID != nil {
		st.responseID = *opts.ResponseID
	}
	if opts.CreatedAt != nil {
		st.createdAt = *opts.CreatedAt
	}
	if opts.RequestMetadata != nil {
		st.metadata = *opts.RequestMetadata
	}
	return st
}

func (st *streamTranslator) createInitialEvent() types.ResponsesStreamEvent {
	resp := types.ResponsesResponse{
		ID:                st.responseID,
		Object:            "response",
		CreatedAt:         st.createdAt,
		Model:             st.model,
		Status:            "in_progress",
		Temperature:       st.metadata.Temperature,
		TopP:              st.metadata.TopP,
		ToolChoice:        st.metadata.ToolChoice,
		Tools:             st.metadata.Tools,
		ParallelToolCalls: new(true),
		Store:             new(true),
		Metadata:          st.metadata.Metadata,
		Output:            []any{},
	}
	if st.metadata.Store != nil {
		resp.Store = st.metadata.Store
	}
	return st.makeEvent("response.created", map[string]any{"response": resp})
}

func (st *streamTranslator) handleChunk(chunk types.OpenAiChatStreamChunk) []types.ResponsesStreamEvent {
	var events []types.ResponsesStreamEvent

	if chunk.Usage != nil {
		if chunk.Usage.PromptTokens != nil {
			st.inputTokens = *chunk.Usage.PromptTokens
		}
		if chunk.Usage.CompletionTokens != nil {
			st.outputTokens = *chunk.Usage.CompletionTokens
		}
		if chunk.Usage.PromptTokensDetails != nil && chunk.Usage.PromptTokensDetails.CachedTokens != nil {
			st.cachedTokens = *chunk.Usage.PromptTokensDetails.CachedTokens
		}
	}

	if len(chunk.Choices) == 0 || chunk.Choices[0].Delta == nil {
		return events
	}
	delta := chunk.Choices[0].Delta

	for _, tc := range delta.ToolCalls {
		idx := tc.Index
		state, ok := st.toolCalls[idx]
		if !ok {
			outputIdx := st.outCounter
			st.outCounter++
			callID := utils.MakeId("call")
			if tc.ID != nil {
				callID = *tc.ID
			}
			item := types.ResponsesOutputFunctionCall{
				ID:     callID,
				Type:   types.ResponsesItemTypeFunctionCall,
				Status: "in_progress",
				CallID: &callID,
			}
			state = &toolCallState{outputIndex: outputIdx, item: item}
			st.toolCalls[idx] = state
			events = append(events, st.makeEvent("response.output_item.added", map[string]any{
				"response_id":  st.responseID,
				"output_index": outputIdx,
				"item":         item,
			}))
		}

		if tc.Function != nil {
			if tc.Function.Name != nil {
				name := ""
				if state.item.Name != nil {
					name = *state.item.Name
				}
				name += *tc.Function.Name
				state.item.Name = &name
			}
			if tc.Function.Arguments != nil {
				partial := ""
				if s, ok := tc.Function.Arguments.(string); ok {
					partial = s
				} else {
					partial = utils.JsonStringifySafe(tc.Function.Arguments)
				}
				if partial != "" {
					args := ""
					if state.item.Arguments != nil {
						args = *state.item.Arguments
					}
					args += partial
					state.item.Arguments = &args
					events = append(events, st.makeEvent("response.function_call_arguments.delta", map[string]any{
						"response_id":  st.responseID,
						"item_id":      state.item.ID,
						"output_index": state.outputIndex,
						"delta":        partial,
					}))
				}
			}
		}
	}

	if delta.Content != nil && *delta.Content != "" {
		if st.textItem == nil {
			idx := st.outCounter
			st.outCounter++
			st.textItemIndex = idx
			st.textItem = &types.ResponsesOutputMessage{
				ID:     utils.MakeId("msg"),
				Type:   types.ResponsesItemTypeMessage,
				Role:   types.OpenAiRoleAssistant,
				Status: "in_progress",
				Content: []map[string]any{
					{"type": "output_text", "text": ""},
				},
			}
			events = append(events, st.makeEvent("response.output_item.added", map[string]any{
				"response_id":  st.responseID,
				"output_index": idx,
				"item":         st.textItem,
			}))
		}
		st.textBuffer += *delta.Content
		st.textItem.Content[0]["text"] = st.textBuffer
		events = append(events, st.makeEvent("response.output_text.delta", map[string]any{
			"response_id":   st.responseID,
			"item_id":       st.textItem.ID,
			"output_index":  st.textItemIndex,
			"content_index": 0,
			"delta":         *delta.Content,
		}))
	}

	return events
}

func (st *streamTranslator) finalize() []types.ResponsesStreamEvent {
	var events []types.ResponsesStreamEvent

	type indexedItem struct {
		index int
		item  any
	}
	var items []indexedItem

	if st.textItem != nil {
		st.textItem.Status = "completed"
		items = append(items, indexedItem{st.textItemIndex, *st.textItem})
	}

	for _, state := range st.toolCalls {
		state.item.Status = "completed"
		name := ""
		if state.item.Name != nil {
			name = *state.item.Name
		}
		if name != "" && SHELL_TOOL_NAMES[name] {
			state.item.Type = types.ResponsesItemTypeLocalShellCall
			var parsed struct {
				Command []string `json:"command"`
			}
			args := ""
			if state.item.Arguments != nil {
				args = *state.item.Arguments
			}
			_ = utils.SafeJsonParse(args, &parsed)
			state.item.Action = &struct {
				Type    *string  `json:"type,omitempty"`
				Command []string `json:"command,omitempty"`
			}{
				Type:    new("exec"),
				Command: parsed.Command,
			}
		}
		items = append(items, indexedItem{state.outputIndex, state.item})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].index < items[j].index
	})

	output := make([]any, 0, len(items))
	for _, it := range items {
		output = append(output, it.item)
		events = append(events, st.makeEvent("response.output_item.done", map[string]any{
			"response_id":  st.responseID,
			"output_index": it.index,
			"item":         it.item,
		}))
	}

	now := time.Now().Unix()
	resp := types.ResponsesResponse{
		ID:                st.responseID,
		Object:            "response",
		CreatedAt:         st.createdAt,
		CompletedAt:       &now,
		Model:             st.model,
		Status:            "completed",
		Temperature:       st.metadata.Temperature,
		TopP:              st.metadata.TopP,
		ToolChoice:        st.metadata.ToolChoice,
		Tools:             st.metadata.Tools,
		ParallelToolCalls: new(true),
		Store:             new(true),
		Metadata:          st.metadata.Metadata,
		Output:            output,
		Usage: &types.ResponsesUsage{
			InputTokens:  st.inputTokens,
			OutputTokens: st.outputTokens,
			TotalTokens:  st.inputTokens + st.outputTokens,
			InputTokensDetails: &struct {
				CachedTokens        *int `json:"cached_tokens,omitempty"`
				CacheCreationTokens *int `json:"cache_creation_tokens,omitempty"`
			}{
				CachedTokens: &st.cachedTokens,
			},
		},
	}
	if st.metadata.Store != nil {
		resp.Store = st.metadata.Store
	}

	events = append(events, st.makeEvent("response.completed", map[string]any{"response": resp}))
	return events
}

func (st *streamTranslator) makeEvent(eventType string, data map[string]any) types.ResponsesStreamEvent {
	st.seq++
	ev := types.ResponsesStreamEvent{
		ID:             utils.MakeId("evt"),
		Object:         "response.event",
		Type:           eventType,
		CreatedAt:      time.Now().Unix(),
		SequenceNumber: st.seq,
		Extra:          data,
	}
	return ev
}
