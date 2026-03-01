package actions

import (
	"testing"
)

func TestDecodeLenient_SimpleFormatFallback(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantLen int
	}{
		{
			name:    "simple action object",
			payload: `{"action": "scope", "path": ".", "notes": "exploring"}`,
			wantLen: 1,
		},
		{
			name:    "markdown fences around simple action",
			payload: "```json\n{\"action\": \"scope\", \"path\": \".\", \"notes\": \"exploring\"}\n```",
			wantLen: 1,
		},
		{
			name:    "markdown fences around full format",
			payload: "```json\n{\"actions\": [{\"type\": \"read_tree\", \"path\": \".\"}], \"notes\": \"note\"}\n```",
			wantLen: 1,
		},
		{
			name:    "simple done action",
			payload: `{"action": "done", "reason": "all work complete"}`,
			wantLen: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env, err := DecodeLenient([]byte(c.payload))
			if err != nil {
				t.Errorf("DecodeLenient(%q) returned error: %v", c.name, err)
				return
			}
			if len(env.Actions) != c.wantLen {
				t.Errorf("got %d actions, want %d", len(env.Actions), c.wantLen)
			}
		})
	}
}
