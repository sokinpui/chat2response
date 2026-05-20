package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
)

type SseMessage struct {
	Event string
	Data  string
}

func ParseSseStream(reader io.Reader) <-chan SseMessage {
	ch := make(chan SseMessage)

	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(reader)

		// SSE blocks are separated by double newlines
		split := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
			if atEOF && len(data) == 0 {
				return 0, nil, nil
			}
			if i := bytes.Index(data, []byte("\n\n")); i >= 0 {
				return i + 2, data[0:i], nil
			}
			if atEOF {
				return len(data), data, nil
			}
			return 0, nil, nil
		}
		scanner.Split(split)

		for scanner.Scan() {
			msg := parseSseBlock(scanner.Text())
			if msg == nil {
				continue
			}
			ch <- *msg
		}
	}()

	return ch
}

func parseSseBlock(block string) *SseMessage {
	var event string
	var dataLines []string

	lines := strings.SplitSeq(block, "\n")
	for line := range lines {
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		field := parts[0]
		value := ""
		if len(parts) > 1 {
			value = strings.TrimPrefix(parts[1], " ")
		}

		switch field {
		case "event":
			event = value
		case "data":
			dataLines = append(dataLines, value)
		}
	}

	if len(dataLines) == 0 {
		return nil
	}

	return &SseMessage{
		Event: event,
		Data:  strings.Join(dataLines, "\n"),
	}
}

func EncodeSseEvent(event string, data any) string {
	payload, ok := data.(string)
	if !ok {
		payload = JsonStringifySafe(data)
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", event, payload)
}
