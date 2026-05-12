package codexappserver

import (
	"testing"

	json "github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	gocmp "github.com/google/go-cmp/cmp"
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
	if diff := gocmp.Diff(wantValue, gotValue); diff != "" {
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
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("decoded params mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGeneratedProtocolTypesRoundTripUnionPayloads(t *testing.T) {
	t.Parallel()

	input := ThreadInjectItemsParams{
		ThreadID: "thr-union-test",
		Items: []jsontext.Value{
			jsontext.Value(`{"type":"tool","name":"git_diff"}`),
			jsontext.Value(`["meta",{"nested":{"type":"agentMessage","text":"hello"}}]`),
		},
	}

	gotBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var got ThreadInjectItemsParams
	if err := json.Unmarshal(gotBytes, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if diff := gocmp.Diff(input, got); diff != "" {
		t.Fatalf("union round-trip mismatch (-want +got):\n%s", diff)
	}
}

func TestGeneratedProtocolTypesDecodeRejectsInvalidDiscriminatorLikePayload(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
	}{
		"error: malformed turn id type is rejected": {
			input: `{"id":123,"status":"inProgress"}`,
		},
		"error: malformed turn status type is rejected": {
			input: `{"id":"turn-1","status":{"value":"inProgress"}}`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got Turn
			if err := json.Unmarshal([]byte(tt.input), &got); err == nil {
				t.Fatal("json.Unmarshal() error = nil, want decode error")
			}
		})
	}
}

func TestGeneratedRequestMethodConstants(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		got  string
		want string
	}{
		"success: initialize": {
			got:  RequestMethodInitialize,
			want: "initialize",
		},
		"success: slash-delimited thread method": {
			got:  RequestMethodThreadMetadataUpdate,
			want: "thread/metadata/update",
		},
		"success: camel-case mcp oauth method": {
			got:  RequestMethodMCPServerOAuthLogin,
			want: "mcpServer/oauth/login",
		},
		"success: alpha9 fuzzy search method": {
			got:  RequestMethodFuzzyFileSearch,
			want: "fuzzyFileSearch",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("request method constant = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestGeneratedNotificationMethodConstants(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		got  string
		want string
	}{
		"success: top-level error method": {
			got:  NotificationMethodError,
			want: "error",
		},
		"success: item agent message delta method": {
			got:  NotificationMethodItemAgentMessageDelta,
			want: "item/agentMessage/delta",
		},
		"success: thread token usage updated method": {
			got:  NotificationMethodThreadTokenUsageUpdated,
			want: "thread/tokenUsage/updated",
		},
		"success: alpha9 fuzzy search completed method": {
			got:  NotificationMethodFuzzyFileSearchSessionCompleted,
			want: "fuzzyFileSearch/sessionCompleted",
		},
		"success: alpha9 windows sandbox setup completed method": {
			got:  NotificationMethodWindowsSandboxSetupCompleted,
			want: "windowsSandbox/setupCompleted",
		},
		"success: deprecated agent message alias": {
			got:  NotificationMethodAgentMessageDelta,
			want: NotificationMethodItemAgentMessageDelta,
		},
		"success: deprecated token usage alias": {
			got:  NotificationMethodThreadTokenUsageUpdatedLegacy,
			want: NotificationMethodThreadTokenUsageUpdated,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("notification method constant = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestGeneratedProtocolClientRequestDecode(t *testing.T) {
	t.Parallel()

	t.Run("success: known method decodes concrete request", func(t *testing.T) {
		t.Parallel()

		got, err := decodeGeneratedClientRequest(jsontext.Value(`{"id":"req-1","method":"` + RequestMethodModelList + `","params":{"includeHidden":true}}`))
		if err != nil {
			t.Fatalf("decodeGeneratedClientRequest() error = %v", err)
		}
		request, ok := got.(ModelListRequest)
		if !ok {
			t.Fatalf("decodeGeneratedClientRequest() = %#v (%T), want ModelListRequest", got, got)
		}
		if request.ID != "req-1" || request.Method != RequestMethodModelList {
			t.Fatalf("decoded request identity = (%q, %q), want (req-1, model/list)", request.ID, request.Method)
		}
		if request.Params.IncludeHidden == nil || !*request.Params.IncludeHidden {
			t.Fatalf("IncludeHidden = %#v, want true pointer", request.Params.IncludeHidden)
		}
	})

	t.Run("success: unknown method preserves raw fallback", func(t *testing.T) {
		t.Parallel()

		got, err := decodeGeneratedClientRequest(jsontext.Value(`{"id":"req-2","method":"future/method","params":{"x":1}}`))
		if err != nil {
			t.Fatalf("decodeGeneratedClientRequest() error = %v", err)
		}
		if _, ok := got.(RawClientRequest); !ok {
			t.Fatalf("decodeGeneratedClientRequest() = %#v (%T), want RawClientRequest", got, got)
		}
	})

	t.Run("error: malformed known request rejects concrete payload", func(t *testing.T) {
		t.Parallel()

		if _, err := decodeGeneratedClientRequest(jsontext.Value(`{"id":123,"method":"` + RequestMethodModelList + `","params":{}}`)); err == nil {
			t.Fatal("decodeGeneratedClientRequest() error = nil, want malformed known request error")
		}
	})
}

var benchmarkGeneratedClientRequest ClientRequest

func BenchmarkGeneratedProtocolClientRequestDecode(b *testing.B) {
	benchmarks := map[string]struct {
		input jsontext.Value
	}{
		"success: known method": {
			input: jsontext.Value(`{"id":"req-1","method":"` + RequestMethodModelList + `","params":{"includeHidden":true}}`),
		},
		"success: unknown method raw fallback": {
			input: jsontext.Value(`{"id":"req-2","method":"future/method","params":{"x":1}}`),
		},
	}
	for name, bm := range benchmarks {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				got, err := decodeGeneratedClientRequest(bm.input)
				if err != nil {
					b.Fatalf("decodeGeneratedClientRequest() error = %v", err)
				}
				benchmarkGeneratedClientRequest = got
			}
		})
	}
}

func TestGeneratedProtocolTypesDecodeInterfaceUnionParity(t *testing.T) {
	t.Parallel()

	t.Run("success: codex error info decodes string and object variants", func(t *testing.T) {
		t.Parallel()

		var stringError TurnError
		if err := json.Unmarshal([]byte(`{"message":"limit","codexErrorInfo":"usageLimitExceeded"}`), &stringError); err != nil {
			t.Fatalf("json.Unmarshal(string codexErrorInfo) error = %v", err)
		}
		if stringError.CodexErrorInfo == nil {
			t.Fatal("CodexErrorInfo = nil, want string variant")
		}
		if got, ok := (*stringError.CodexErrorInfo).(CodexErrorInfoValue); !ok || got != CodexErrorInfoValueUsageLimitExceeded {
			t.Fatalf("CodexErrorInfo = %#v (%T), want %s", *stringError.CodexErrorInfo, *stringError.CodexErrorInfo, CodexErrorInfoValueUsageLimitExceeded)
		}

		var objectError TurnError
		input := `{"message":"steer","codexErrorInfo":{"activeTurnNotSteerable":{"turnKind":"review"}}}`
		if err := json.Unmarshal([]byte(input), &objectError); err != nil {
			t.Fatalf("json.Unmarshal(object codexErrorInfo) error = %v", err)
		}
		if objectError.CodexErrorInfo == nil {
			t.Fatal("CodexErrorInfo = nil, want object variant")
		}
		if _, ok := (*objectError.CodexErrorInfo).(ActiveTurnNotSteerableCodexErrorInfo); !ok {
			t.Fatalf("CodexErrorInfo = %#v (%T), want ActiveTurnNotSteerableCodexErrorInfo", *objectError.CodexErrorInfo, *objectError.CodexErrorInfo)
		}
	})

	t.Run("success: unknown codex error info preserves raw fallback", func(t *testing.T) {
		t.Parallel()

		var got TurnError
		if err := json.Unmarshal([]byte(`{"message":"future","codexErrorInfo":{"futureCode":true}}`), &got); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if got.CodexErrorInfo == nil {
			t.Fatal("CodexErrorInfo = nil, want raw fallback")
		}
		if _, ok := (*got.CodexErrorInfo).(RawCodexErrorInfo); !ok {
			t.Fatalf("CodexErrorInfo = %#v (%T), want RawCodexErrorInfo", *got.CodexErrorInfo, *got.CodexErrorInfo)
		}
	})

	t.Run("success: reasoning summary decodes generated value variant", func(t *testing.T) {
		t.Parallel()

		var got TurnStartParams
		if err := json.Unmarshal([]byte(`{"threadId":"thr-1","input":[],"summary":"none"}`), &got); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if got.Summary == nil {
			t.Fatal("Summary = nil, want value variant")
		}
		if summary, ok := (*got.Summary).(ReasoningSummaryValue); !ok || summary != ReasoningSummaryValueNone {
			t.Fatalf("Summary = %#v (%T), want %s", *got.Summary, *got.Summary, ReasoningSummaryValueNone)
		}
	})

	t.Run("success: session source decodes nested sub-agent variants", func(t *testing.T) {
		t.Parallel()

		input := `{
			"cliVersion":"0.1.0",
			"createdAt":1,
			"cwd":"/tmp/project",
			"ephemeral":false,
			"id":"thr-1",
			"modelProvider":"openai",
			"preview":"",
			"sessionId":"sess-1",
			"source":{"subAgent":{"other":"external"}},
			"status":{"type":"idle"},
			"turns":[],
			"updatedAt":2
		}`
		var got ThreadPayload
		if err := json.Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		source, ok := got.Source.(SubAgentSessionSource)
		if !ok {
			t.Fatalf("Source = %#v (%T), want SubAgentSessionSource", got.Source, got.Source)
		}
		if subAgent, ok := source.SubAgent.(OtherSubAgentSource); !ok || subAgent.Other != "external" {
			t.Fatalf("SubAgent = %#v (%T), want OtherSubAgentSource", source.SubAgent, source.SubAgent)
		}
	})
}
