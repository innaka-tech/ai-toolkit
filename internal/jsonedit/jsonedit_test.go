package jsonedit

import "testing"

func TestTrailingContentIsRefused(t *testing.T) {
	for _, in := range []string{`{"a":1} // comment`, `{"a":1}{"b":2}`, `{"a":1} x`} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
	if _, err := Parse([]byte("{\"a\":1}\n\n")); err != nil {
		t.Errorf("trailing whitespace refused: %v", err)
	}
}
