package protocol

import (
	"testing"

	json "github.com/go-json-experiment/json"
	"github.com/google/go-cmp/cmp"
)

func TestGeneratedProtocolTypesJSON(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value any
		want  string
	}{
		"success: command exec params preserve wire fields": {
			value: CommandExecParams{
				Command:            []string{"printf", "hello"},
				Cwd:                new("/tmp/work"),
				Env:                map[string]*string{"EMPTY": nil, "FOO": new("bar")},
				StreamStdoutStderr: new(true),
				TimeoutMs:          new(int64(2500)),
			},
			want: `{"command":["printf","hello"],"cwd":"/tmp/work","env":{"EMPTY":null,"FOO":"bar"},"streamStdoutStderr":true,"timeoutMs":2500}`,
		},
		"success: fs read file response uses base64 field": {
			value: FsReadFileResponse{DataBase64: "aGVsbG8="},
			want:  `{"dataBase64":"aGVsbG8="}`,
		},
		"success: enum constants encode as strings": {
			value: ReasoningEffortHigh,
			want:  `"high"`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			assertJSONEqual(t, tt.want, got)
		})
	}
}

func assertJSONEqual(t *testing.T, want string, got []byte) {
	t.Helper()

	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("json.Unmarshal(want) error = %v", err)
	}
	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("json.Unmarshal(got) error = %v; got %s", err, got)
	}
	if diff := cmp.Diff(wantValue, gotValue); diff != "" {
		t.Fatalf("json output mismatch (-want +got):\n%s\nraw got: %s", diff, got)
	}
}

func TestGeneratedProtocolTypesDecode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		want  CommandExecParams
	}{
		"success: optional nullable fields decode into pointers and maps": {
			input: `{"command":["echo","ok"],"disableTimeout":true,"env":{"FOO":"bar","REMOVE":null},"timeoutMs":123}`,
			want: CommandExecParams{
				Command:        []string{"echo", "ok"},
				DisableTimeout: new(true),
				Env:            map[string]*string{"FOO": new("bar"), "REMOVE": nil},
				TimeoutMs:      new(int64(123)),
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got CommandExecParams
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("decoded params mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
