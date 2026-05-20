package anthropic

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

type blockState struct {
	btype       string
	outputIndex int
	item        any
	buffer      string
}

type streamTranslator struct {
	model      string
	responseID string
	createdAt  int64
	metadata   ResponsesStreamMetadata
	seq        int
	outCounter int
	blocks     map[int]*blockState

	inputTokens         int
	outputTokens        int
	cacheCreationTokens int
	cacheReadTokens     int

	textItem      *types.ResponsesOutputMessage
	textItemIndex int
	textBuffer    string

	stopReason *string
}

func TranslateStream(stream <-chan utils.SseMessage, options TranslateStreamOptions) <-chan types.ResponsesStreamEvent {
	out := make(chan types.ResponsesStreamEvent)
	go func() {
		defer close(out)
		translator := newStreamTranslator(options)
		out <- translator.createInitialEvent()

		for msg := range stream {
			var event types.AnthropicStreamEvent
			if err := utils.SafeJsonParse(msg.Data, &event); err != nil {
				continue
			}
			for _, ev := range translator.handleEvent(event) {
				out <- ev
			}
		}

		for _, ev := range translator.finalize() {
			out <- ev
		}
	}()
	return out
}

func newStreamTranslator(opts TranslateStreamOptions) *streamTranslator {
	st := &streamTranslator{
		model:         "",
		responseID:    utils.MakeId("resp"),
		createdAt:     time.Now().Unix(),
		blocks:        make(map[int]*blockState),
		textItemIndex: -1,
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

func (st *streamTranslator) handleEvent(event types.AnthropicStreamEvent) []types.ResponsesStreamEvent {
	var events []types.ResponsesStreamEvent

	switch event.Type {
	case "message_start":
		if event.Message != nil {
			st.inputTokens = event.Message.Usage.InputTokens
			if event.Message.Usage.CacheCreationInputTokens != nil {
				st.cacheCreationTokens = *event.Message.Usage.CacheCreationInputTokens
			}
			if event.Message.Usage.CacheReadInputTokens != nil {
				st.cacheReadTokens = *event.Message.Usage.CacheReadInputTokens
			}
		}

	case "content_block_start":
		if event.Index != nil && event.ContentBlock != nil {
			events = append(events, st.onContentBlockStart(*event.Index, *event.ContentBlock)...)
		}

	case "content_block_delta":
		if event.Index != nil && event.Delta != nil {
			events = append(events, st.onContentBlockDelta(*event.Index, event.Delta)...)
		}

	case "message_delta":
		if event.Usage != nil {
			st.outputTokens = event.Usage.OutputTokens
		}
		if stop, ok := event.Delta["stop_reason"].(string); ok {
			st.stopReason = &stop
		}
	}

	return events
}

func (st *streamTranslator) onContentBlockStart(index int, block types.AnthropicContentBlock) []types.ResponsesStreamEvent {
	var events []types.ResponsesStreamEvent

	switch block.Type {
	case "thinking":
		idx := st.outCounter
		st.outCounter++
		item := types.ResponsesOutputReasoning{
			ID:      utils.MakeId("rs"),
			Type:    types.ResponsesItemTypeReasoning,
			Summary: []any{},
			Content: []types.ResponsesContentPart{{Type: "reasoning_text", Text: new("")}},
			Status:  new("in_progress"),
		}
		st.blocks[index] = &blockState{btype: "thinking", outputIndex: idx, item: item}
		events = append(events, st.makeEvent("response.output_item.added", map[string]any{
			"response_id":  st.responseID,
			"output_index": idx,
			"item":         item,
		}))

	case "text":
		if st.textItem == nil {
			idx := st.outCounter
			st.outCounter++
			st.textItemIndex = idx
			st.textItem = &types.ResponsesOutputMessage{
				ID:     utils.MakeId("msg"),
				Type:   types.ResponsesItemTypeMessage,
				Role:   "assistant",
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
		st.blocks[index] = &blockState{btype: "text", outputIndex: st.textItemIndex}

	case "tool_use":
		idx := st.outCounter
		st.outCounter++
		callID := utils.MakeId("call")
		if block.ID != nil {
			callID = *block.ID
		}
		name := ""
		if block.Name != nil {
			name = *block.Name
		}
		item := types.ResponsesOutputFunctionCall{
			ID:        callID,
			Type:      types.ResponsesItemTypeFunctionCall,
			Status:    "in_progress",
			Name:      &name,
			Arguments: new(""),
			CallID:    &callID,
		}
		st.blocks[index] = &blockState{btype: "tool_use", outputIndex: idx, item: item}
		events = append(events, st.makeEvent("response.output_item.added", map[string]any{
			"response_id":  st.responseID,
			"output_index": idx,
			"item":         item,
		}))
	}

	return events
}

func (st *streamTranslator) onContentBlockDelta(index int, delta map[string]any) []types.ResponsesStreamEvent {
	var events []types.ResponsesStreamEvent
	block := st.blocks[index]
	if block == nil {
		return nil
	}

	dtype, _ := delta["type"].(string)

	switch dtype {
	case "text_delta":
		text, _ := delta["text"].(string)
		if text == "" {
			return nil
		}
		st.textBuffer += text
		events = append(events, st.makeEvent("response.output_text.delta", map[string]any{
			"response_id":   st.responseID,
			"item_id":       st.textItem.ID,
			"output_index":  st.textItemIndex,
			"content_index": 0,
			"delta":         text,
		}))

	case "thinking_delta":
		thinking, _ := delta["thinking"].(string)
		if thinking == "" {
			return nil
		}
		block.buffer += thinking
		if item, ok := block.item.(types.ResponsesOutputReasoning); ok {
			text := block.buffer
			item.Content[0].Text = &text
			events = append(events, st.makeEvent("response.reasoning_text.delta", map[string]any{
				"response_id":   st.responseID,
				"item_id":       item.ID,
				"output_index":  block.outputIndex,
				"content_index": 0,
				"delta":         thinking,
			}))
		}

	case "input_json_delta":
		partial, _ := delta["partial_json"].(string)
		if partial == "" {
			return nil
		}
		block.buffer += partial
		if item, ok := block.item.(types.ResponsesOutputFunctionCall); ok {
			args := block.buffer
			item.Arguments = &args
			events = append(events, st.makeEvent("response.function_call_arguments.delta", map[string]any{
				"response_id":  st.responseID,
				"item_id":      item.ID,
				"output_index": block.outputIndex,
				"delta":        partial,
			}))
		}
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
		st.textItem.Content[0]["text"] = st.textBuffer
		items = append(items, indexedItem{st.textItemIndex, *st.textItem})
	}

	for _, block := range st.blocks {
		if block.item == nil {
			continue
		}
		alreadyAdded := false
		for _, it := range items {
			if it.index == block.outputIndex {
				alreadyAdded = true
				break
			}
		}
		if alreadyAdded {
			continue
		}

		if block.btype == "thinking" {
			if item, ok := block.item.(types.ResponsesOutputReasoning); ok {
				item.Status = new("completed")
				items = append(items, indexedItem{block.outputIndex, item})
			}
		} else if block.btype == "tool_use" {
			if item, ok := block.item.(types.ResponsesOutputFunctionCall); ok {
				item.Status = "completed"
				if item.Name != nil && shellToolNames[*item.Name] {
					item.Type = types.ResponsesItemTypeLocalShellCall
					var parsed struct {
						Command []string `json:"command"`
					}
					_ = utils.SafeJsonParse(*item.Arguments, &parsed)
					item.Action = &struct {
						Type    *string  `json:"type,omitempty"`
						Command []string `json:"command,omitempty"`
					}{
						Type:    new("exec"),
						Command: parsed.Command,
					}
				}
				items = append(items, indexedItem{block.outputIndex, item})
			}
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].index < items[j].index
	})

	output := make([]any, len(items))
	for i, it := range items {
		output[i] = it.item
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
				CachedTokens:        &st.cacheReadTokens,
				CacheCreationTokens: &st.cacheCreationTokens,
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
	return types.ResponsesStreamEvent{
		ID:             utils.MakeId("evt"),
		Object:         "response.event",
		Type:           eventType,
		CreatedAt:      time.Now().Unix(),
		SequenceNumber: st.seq,
		Extra:          data,
	}
}
