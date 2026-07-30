package main

import (
	"encoding/json"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// ActivityCallback is the presentation-only side channel for meaningful pi
// activity. It is deliberately separate from the watcher Event stream.
type ActivityCallback func(string)

const (
	maxPiParserLineBytes = 64 * 1024
	maxPiActivityRunes   = 512
)

type piActivityParser struct {
	line        []byte
	oversized   bool
	activity    ActivityCallback
	toolSummary map[string]string
}

func newPiActivityParser(activity ActivityCallback) *piActivityParser {
	return &piActivityParser{activity: activity, toolSummary: map[string]string{}}
}

// Write consumes output without retaining more than one bounded line. It is
// intentionally infallible: parser failures must never affect the agent.
func (p *piActivityParser) Write(data []byte) (int, error) {
	for _, b := range data {
		if b == '\n' {
			p.finishLine()
			continue
		}
		if p.oversized {
			continue
		}
		if len(p.line) == maxPiParserLineBytes {
			p.oversized = true
			p.line = nil
			continue
		}
		p.line = append(p.line, b)
	}
	return len(data), nil
}

func (p *piActivityParser) Flush() {
	if p.oversized {
		p.line = nil
		p.oversized = false
		return
	}
	if len(p.line) > 0 {
		p.finishLine()
	}
}

func (p *piActivityParser) finishLine() {
	if p.oversized {
		p.line = nil
		p.oversized = false
		return
	}
	line := strings.TrimSuffix(string(p.line), "\r")
	p.line = nil
	if line == "" {
		return
	}
	p.parseLine(line)
}

func (p *piActivityParser) emit(text string) {
	if p.activity == nil {
		return
	}
	if text = sanitizePiActivity(text); text != "" {
		p.activity(text)
	}
}

func (p *piActivityParser) parseLine(line string) {
	var event map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		p.emit(line)
		return
	}
	eventType := jsonString(event, "type")
	if eventType == "" {
		return
	}

	// Pi emits completed assistant narratives as message_end snapshots.
	// The direct completion forms are accepted as well so a small pi
	// protocol change does not turn useful text into a silent ticker.
	switch eventType {
	case "message_end":
		if message, ok := event["message"]; ok {
			var snapshot map[string]json.RawMessage
			if json.Unmarshal(message, &snapshot) == nil && jsonString(snapshot, "role") == "assistant" {
				p.emit(extractText(snapshot["content"]))
			}
		}
		return
	case "text_complete", "text_end", "assistant_text":
		p.emit(jsonString(event, "text"))
		if jsonString(event, "text") == "" {
			p.emit(jsonString(event, "content"))
		}
		return
	case "message_update":
		var update map[string]json.RawMessage
		if json.Unmarshal(event["assistantMessageEvent"], &update) == nil {
			t := jsonString(update, "type")
			if t == "text_end" || t == "text_complete" {
				p.emit(jsonString(update, "text"))
				if jsonString(update, "text") == "" {
					p.emit(jsonString(update, "content"))
				}
			}
		}
		return
	case "tool_execution_start":
		p.toolStart(event)
		return
	case "tool_execution_end", "tool_execution_complete", "tool_result":
		p.toolEnd(event)
		return
	default:
		// Token deltas, thinking, snapshots, progress, and unknown JSON
		// events are intentionally silent.
		return
	}
}

func (p *piActivityParser) toolStart(event map[string]json.RawMessage) {
	id := firstJSONString(event, "toolCallId", "tool_call_id", "id")
	name := firstJSONString(event, "toolName", "tool_name", "name")
	if name == "" {
		return
	}
	summary := name
	if arg := toolArgument(name, event["args"]); arg != "" {
		summary += ": " + arg
	}
	if id != "" {
		p.toolSummary[id] = summary
	}
	p.emit("▶ " + summary)
}

func (p *piActivityParser) toolEnd(event map[string]json.RawMessage) {
	id := firstJSONString(event, "toolCallId", "tool_call_id", "id")
	summary := p.toolSummary[id]
	if summary == "" {
		name := firstJSONString(event, "toolName", "tool_name", "name")
		if name != "" {
			summary = name
		}
	}
	if summary == "" {
		return
	}
	failed := jsonBool(event, "isError") || jsonBool(event, "is_error")
	diagnostic := firstJSONString(event, "error", "message")
	if diagnostic == "" {
		diagnostic = extractDiagnostic(event["error"])
	}
	if failed {
		if diagnostic != "" {
			summary += ": " + diagnostic
		}
		p.emit("✗ " + summary)
	} else {
		p.emit("✓ " + summary)
	}
	if id != "" {
		delete(p.toolSummary, id)
	}
}

func toolArgument(name string, raw json.RawMessage) string {
	var args map[string]json.RawMessage
	if json.Unmarshal(raw, &args) != nil {
		return ""
	}
	key := ""
	switch strings.ToLower(name) {
	case "bash":
		key = "command"
	case "read", "edit", "write":
		key = "path"
	}
	if key == "" {
		return ""
	}
	return jsonString(args, key)
}

func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var parts []map[string]json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	for _, part := range parts {
		if jsonString(part, "type") == "text" {
			text += jsonString(part, "text")
		}
	}
	return text
}

func extractDiagnostic(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if varString := jsonStringValue(raw); varString != "" {
		return varString
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) == nil {
		for _, key := range []string{"message", "error", "text", "content"} {
			if value := jsonString(object, key); value != "" {
				return value
			}
			if value := extractText(object[key]); value != "" {
				return value
			}
		}
	}
	return ""
}

func jsonString(event map[string]json.RawMessage, key string) string {
	return jsonStringValue(event[key])
}

func firstJSONString(event map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if value := jsonString(event, key); value != "" {
			return value
		}
	}
	return ""
}

func jsonStringValue(raw json.RawMessage) string {
	var value string
	if len(raw) != 0 && json.Unmarshal(raw, &value) == nil {
		return value
	}
	return ""
}

func jsonBool(event map[string]json.RawMessage, key string) bool {
	var value bool
	return json.Unmarshal(event[key], &value) == nil && value
}

// sanitizePiActivity removes terminal escape/control sequences, collapses
// whitespace, and bounds retained display text. The raw invocation stream is
// sanitized nowhere; this function only runs on the presentation side.
func sanitizePiActivity(text string) string {
	text = ansi.Strip(text)
	var clean strings.Builder
	clean.Grow(len(text))
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if unicode.IsControl(r) {
			if unicode.IsSpace(r) {
				clean.WriteByte(' ')
			}
			i += size
			continue
		}
		clean.WriteRune(r)
		i += size
	}
	words := strings.Fields(clean.String())
	if len(words) == 0 {
		return ""
	}
	text = strings.Join(words, " ")
	if runes := []rune(text); len(runes) > maxPiActivityRunes {
		text = string(runes[:maxPiActivityRunes])
	}
	return text
}

type piActivitySink struct {
	file   io.Writer
	parser *piActivityParser
}

func (s *piActivitySink) Write(p []byte) (int, error) {
	n, err := s.file.Write(p)
	if err != nil || n != len(p) {
		return n, err
	}
	_, _ = s.parser.Write(p)
	return n, nil
}
