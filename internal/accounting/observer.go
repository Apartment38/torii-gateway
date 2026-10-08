package accounting

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// Observer inspects bounded copies while forwarding every original byte. Usage
// frames may span arbitrary reads. Oversized frames/bodies retain estimates.
type Observer struct {
	stream                  bool
	buffer                  []byte
	data                    []byte
	overflow                bool
	Input, Output           int64
	inputKnown, outputKnown bool
	Reported                bool
	completed               bool
	total                   int64
	totalKnown, noOutput    bool
}

func NewObserver(stream bool) *Observer { return &Observer{stream: stream} }

const observationLimit = 2 << 20

func (o *Observer) Write(p []byte) (int, error) {
	n := len(p)
	if !o.stream {
		if len(o.buffer)+n <= observationLimit {
			o.buffer = append(o.buffer, p...)
		} else {
			o.overflow = true
			o.outputKnown = false
			o.totalKnown = false
		}
		return n, nil
	}
	for _, b := range p {
		if b == '\n' {
			o.line(o.buffer)
			o.buffer = o.buffer[:0]
		} else if len(o.buffer) < observationLimit {
			o.buffer = append(o.buffer, b)
		} else {
			o.overflow = true
			o.outputKnown = false
			o.totalKnown = false
		}
	}
	return n, nil
}
func (o *Observer) line(line []byte) {
	line = bytes.TrimSuffix(line, []byte{'\r'})
	if bytes.HasPrefix(line, []byte("event:")) {
		kind := strings.TrimSpace(string(bytes.TrimPrefix(line, []byte("event:"))))
		if kind == "response.completed" || kind == "message_stop" {
			o.completed = true
		}
	}
	if len(line) == 0 {
		if !o.overflow {
			o.parse(o.data)
		}
		o.data = o.data[:0]
		o.overflow = false
		return
	}
	if bytes.HasPrefix(line, []byte("data:")) {
		value := bytes.TrimPrefix(line, []byte("data:"))
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
		if len(o.data)+len(value)+1 <= observationLimit {
			o.data = append(o.data, value...)
			o.data = append(o.data, '\n')
		} else {
			o.overflow = true
			o.outputKnown = false
			o.totalKnown = false
		}
	}
}
func (o *Observer) Finish() {
	if !o.stream {
		if !o.overflow {
			o.parse(o.buffer)
		}
	} else {
		if len(o.buffer) > 0 {
			o.line(o.buffer)
		}
		if !o.overflow {
			o.parse(o.data)
		}
	}
	if o.noOutput && !o.outputKnown {
		o.outputKnown = true
	}
	if o.totalKnown {
		if o.inputKnown && !o.outputKnown && o.total >= o.Input {
			o.Output = o.total - o.Input
			o.outputKnown = true
		} else if o.outputKnown && !o.inputKnown && o.total >= o.Output {
			o.Input = o.total - o.Output
			o.inputKnown = true
		}
	}
	o.Reported = o.inputKnown && o.outputKnown && (!o.totalKnown || o.Input+o.Output == o.total) && (!o.stream || o.completed)
	o.buffer = nil
	o.data = nil
}
func (o *Observer) parse(data []byte) { o.parseUsage(data, 0) }
func (o *Observer) parseUsage(data []byte, depth int) {
	if depth > 2 {
		o.outputKnown = false
		o.totalKnown = false
		return
	}
	if strings.TrimSpace(string(data)) == "[DONE]" {
		o.completed = true
		return
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		if o.stream && len(bytes.TrimSpace(data)) > 0 {
			o.outputKnown = false
			o.totalKnown = false
		}
		return
	}
	if o.stream {
		var kind string
		json.Unmarshal(root["type"], &kind)
		if kind == "response.completed" || kind == "message_stop" {
			o.completed = true
		}
		if generatedData(root) {
			o.outputKnown = false
			o.totalKnown = false
		}
	}
	for _, nested := range []string{"response", "message"} {
		if raw := root[nested]; len(raw) > 0 {
			o.parseUsage(raw, depth+1)
		}
	}
	raw := root["usage"]
	if len(raw) == 0 {
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return
	}
	get := func(names ...string) (int64, bool) {
		for _, name := range names {
			if raw, ok := fields[name]; ok {
				var value int64
				if json.Unmarshal(raw, &value) == nil && value >= 0 && value <= 1e12 {
					return value, true
				}
			}
		}
		return 0, false
	}
	if v, ok := get("prompt_tokens", "input_tokens"); ok {
		o.Input = max(o.Input, v)
		o.inputKnown = true
	}
	if v, ok := get("completion_tokens", "output_tokens"); ok {
		o.Output = max(o.Output, v)
		o.outputKnown = true
	}
	if v, ok := get("total_tokens"); ok {
		o.total = max(o.total, v)
		o.totalKnown = true
	}

}

// Any generation after a usage snapshot makes that output count provisional
// until a subsequent usage frame arrives. This catches missing final usage.
func generatedData(root map[string]json.RawMessage) bool {
	nonempty := func(raw json.RawMessage) bool {
		value := strings.TrimSpace(string(raw))
		return value != "" && value != "null" && value != "{}" && value != "[]" && value != `""`
	}
	for _, key := range []string{"delta", "text", "content", "output"} {
		if nonempty(root[key]) {
			return true
		}
	}
	var choices []map[string]json.RawMessage
	if json.Unmarshal(root["choices"], &choices) == nil {
		for _, choice := range choices {
			if nonempty(choice["delta"]) || nonempty(choice["text"]) || nonempty(choice["message"]) {
				return true
			}
		}
	}
	return false
}

type observedBody struct {
	io.ReadCloser
	observer *Observer
	done     bool
	complete func(*Observer, bool)
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.observer.Write(p[:n])
	if err != nil {
		b.finish(err == io.EOF)
	}
	return n, err
}
func (b *observedBody) Close() error { err := b.ReadCloser.Close(); b.finish(false); return err }
func (b *observedBody) finish(eof bool) {
	if b.done {
		return
	}
	b.done = true
	b.observer.Finish()
	b.complete(b.observer, eof)
}
