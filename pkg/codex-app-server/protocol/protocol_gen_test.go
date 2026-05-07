package protocol

import (
	"testing"

	json "github.com/go-json-experiment/json"
	"github.com/google/go-cmp/cmp"
)

func TestGeneratedProtocolTypesJSON(t *testing.T) {
	tests := map[string]struct {
		value any
		want  string
	}{
		"success: command exec params preserve wire fields": {
			value: CommandExecParams{
				Command:            []string{"printf", "hello"},
				Cwd:                stringPtr("/tmp/work"),
				Env:                map[string]*string{"EMPTY": nil, "FOO": stringPtr("bar")},
				StreamStdoutStderr: boolPtr(true),
				TimeoutMs:          int64Ptr(2500),
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
			got, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, string(got)); diff != "" {
				t.Fatalf("json output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGeneratedProtocolTypesDecode(t *testing.T) {
	tests := map[string]struct {
		input string
		want  CommandExecParams
	}{
		"success: optional nullable fields decode into pointers and maps": {
			input: `{"command":["echo","ok"],"disableTimeout":true,"env":{"FOO":"bar","REMOVE":null},"timeoutMs":123}`,
			want: CommandExecParams{
				Command:        []string{"echo", "ok"},
				DisableTimeout: boolPtr(true),
				Env:            map[string]*string{"FOO": stringPtr("bar"), "REMOVE": nil},
				TimeoutMs:      int64Ptr(123),
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
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

func stringPtr(value string) *string { return &value }
func boolPtr(value bool) *bool       { return &value }
func int64Ptr(value int64) *int64    { return &value }
