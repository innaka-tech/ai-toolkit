// Package jsonedit edits JSON objects while preserving key order and untouched values,
// so aitk can update tool config files without reformatting the user's settings.
package jsonedit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Object is a JSON object with ordered keys. Values stay raw until edited.
type Object struct {
	keys []string
	vals map[string]json.RawMessage
}

// Parse decodes an object. Empty input yields an empty object. JSONC line comments are rejected.
func Parse(b []byte) (*Object, error) {
	o := &Object{vals: map[string]json.RawMessage{}}
	if len(bytes.TrimSpace(b)) == 0 {
		return o, nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("top-level value is not an object")
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		k := kt.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		if _, dup := o.vals[k]; !dup {
			o.keys = append(o.keys, k)
		}
		o.vals[k] = raw
	}
	if _, err := dec.Token(); err != nil { // closing brace
		return nil, err
	}
	if rest := bytes.TrimSpace(b[dec.InputOffset():]); len(rest) > 0 {
		return nil, fmt.Errorf("unexpected content after the JSON object")
	}
	return o, nil
}

// Get returns the raw value at key.
func (o *Object) Get(k string) (json.RawMessage, bool) { v, ok := o.vals[k]; return v, ok }

// Set stores a value (marshaled) at key, keeping the key's position if it exists.
func (o *Object) Set(k string, v any) error {
	raw, err := marshal(v)
	if err != nil {
		return err
	}
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = raw
	return nil
}

// Delete removes key, keeping the order of the others.
func (o *Object) Delete(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, x := range o.keys {
		if x == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// Len is the number of keys.
func (o *Object) Len() int { return len(o.keys) }

// Child returns the object at key (creating it when absent).
func (o *Object) Child(k string) (*Object, error) {
	raw, ok := o.vals[k]
	if !ok || string(bytes.TrimSpace(raw)) == "null" {
		return &Object{vals: map[string]json.RawMessage{}}, nil
	}
	c, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", k, err)
	}
	return c, nil
}

// MarshalJSON renders the object with keys in order.
func (o *Object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		b.Write(o.vals[k])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Bytes renders the object indented with two spaces and a trailing newline.
func (o *Object) Bytes() []byte {
	raw, _ := o.MarshalJSON()
	var out bytes.Buffer
	json.Indent(&out, raw, "", "  ")
	out.WriteByte('\n')
	return out.Bytes()
}

func marshal(v any) (json.RawMessage, error) {
	if o, ok := v.(*Object); ok {
		return o.MarshalJSON()
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return json.RawMessage(strings.TrimSpace(b.String())), nil
}
