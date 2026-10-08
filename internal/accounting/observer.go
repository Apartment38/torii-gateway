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
		}
	}
	return n, nil
}
func (o *Observer) line(line []byte) {
	line = bytes.TrimSuffix(line, []byte{'\r'})
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
	o.Reported = o.inputKnown && o.outputKnown
	o.buffer = nil
	o.data = nil
}
func (o *Observer) parse(data []byte) {
	if strings.TrimSpace(string(data)) == "[DONE]" {
		return
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return
	}
	for _, nested := range []string{"response", "message"} {
		if raw := root[nested]; len(raw) > 0 {
			o.parse(raw)
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
	if _, ok := fields["total_tokens"]; ok && !o.inputKnown {
		if v, ok := get("total_tokens"); ok {
			o.Input = v
			o.inputKnown = true
			o.outputKnown = true
		}
	}
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
