// Package schema validates documents against the embedded JSON Schemas.
package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/innaka-tech/ai-toolkit/schemas"
)

const base = "https://raw.githubusercontent.com/innaka-tech/ai-toolkit/main/schemas/"

var (
	once     sync.Once
	compiled map[string]*jsonschema.Schema
	initErr  error
)

func load() {
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	entries, err := schemas.FS.ReadDir(".")
	if err != nil {
		initErr = err
		return
	}
	for _, e := range entries {
		b, _ := schemas.FS.ReadFile(e.Name())
		doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
		if err != nil {
			initErr = fmt.Errorf("%s: %w", e.Name(), err)
			return
		}
		if err := c.AddResource(base+e.Name(), doc); err != nil {
			initErr = err
			return
		}
	}
	compiled = map[string]*jsonschema.Schema{}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".schema.json")
		s, err := c.Compile(base + e.Name())
		if err != nil {
			initErr = fmt.Errorf("%s: %w", e.Name(), err)
			return
		}
		compiled[name] = s
	}
}

// Validate checks v (any JSON-marshalable value) against schema name (e.g. "task").
func Validate(name string, v any) error {
	once.Do(load)
	if initErr != nil {
		return initErr
	}
	s, ok := compiled[name]
	if !ok {
		return fmt.Errorf("unknown schema %q", name)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	if err := s.Validate(inst); err != nil {
		return fmt.Errorf("%s schema: %s", name, flatten(err))
	}
	return nil
}

var printer = message.NewPrinter(language.English)

func flatten(err error) string {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return err.Error()
	}
	var msgs []string
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			loc := "/" + strings.Join(e.InstanceLocation, "/")
			msgs = append(msgs, fmt.Sprintf("%s: %s", loc, e.ErrorKind.LocalizedString(printer)))
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	return strings.Join(msgs, "; ")
}
