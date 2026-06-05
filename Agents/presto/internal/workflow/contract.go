package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type JSONKind string

const (
	KindAny    JSONKind = "any"
	KindString JSONKind = "string"
	KindNumber JSONKind = "number"
	KindBool   JSONKind = "bool"
	KindObject JSONKind = "object"
	KindArray  JSONKind = "array"
)

type FieldSpec struct {
	Kind     JSONKind `json:"kind"`
	Required bool     `json:"required,omitempty"`
}

type OutputContract struct {
	Fields      map[string]FieldSpec `json:"fields,omitempty"`
	MaxAttempts int                  `json:"max_attempts,omitempty"`
	Strict      bool                 `json:"strict,omitempty"`
}

func ObjectContract(fields map[string]FieldSpec) OutputContract {
	return OutputContract{
		Fields:      fields,
		MaxAttempts: 2,
		Strict:      true,
	}
}

func Required(kind JSONKind) FieldSpec {
	return FieldSpec{Kind: kind, Required: true}
}

func Optional(kind JSONKind) FieldSpec {
	return FieldSpec{Kind: kind}
}

func (c OutputContract) Enabled() bool {
	return len(c.Fields) > 0
}

func (c OutputContract) Validate(output string) (json.RawMessage, error) {
	if !c.Enabled() {
		return nil, nil
	}

	raw, err := extractJSONObject(output, c.Strict)
	if err != nil {
		return nil, err
	}

	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("output is not a JSON object: %w", err)
	}

	for name, spec := range c.Fields {
		value, ok := object[name]
		if !ok {
			if spec.Required {
				return nil, fmt.Errorf("missing required field %q", name)
			}
			continue
		}
		if err := validateKind(name, value, spec.Kind); err != nil {
			return nil, err
		}
	}
	if c.Strict {
		for name := range object {
			if _, ok := c.Fields[name]; !ok {
				return nil, fmt.Errorf("unexpected field %q", name)
			}
		}
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), compact.Bytes()...), nil
}

func extractJSONObject(output string, strict bool) ([]byte, error) {
	text := strings.TrimSpace(output)
	if text == "" {
		return nil, errors.New("empty output")
	}
	if !strict && strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) >= 3 {
			text = strings.Join(lines[1:len(lines)-1], "\n")
			text = strings.TrimSpace(text)
		}
	}
	if strict {
		var raw json.RawMessage
		decoder := json.NewDecoder(strings.NewReader(text))
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("output is not valid JSON: %w", err)
		}
		if decoder.Decode(&json.RawMessage{}) != io.EOF {
			return nil, errors.New("strict output must contain only JSON")
		}
		return append([]byte(nil), raw...), nil
	}
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end < start {
		return nil, errors.New("output does not contain a JSON object")
	}
	return []byte(text[start : end+1]), nil
}

func validateKind(name string, value any, kind JSONKind) error {
	switch kind {
	case "", KindAny:
		return nil
	case KindString:
		if _, ok := value.(string); ok {
			return nil
		}
	case KindNumber:
		if _, ok := value.(float64); ok {
			return nil
		}
	case KindBool:
		if _, ok := value.(bool); ok {
			return nil
		}
	case KindObject:
		if _, ok := value.(map[string]any); ok {
			return nil
		}
	case KindArray:
		if _, ok := value.([]any); ok {
			return nil
		}
	default:
		return fmt.Errorf("field %q uses unknown kind %q", name, kind)
	}
	return fmt.Errorf("field %q must be %s", name, kind)
}
