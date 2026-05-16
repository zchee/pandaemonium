// Copyright 2026 The pandaemonium Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package toml

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestFacadeUnmarshalScalarsTablesAndArrays(t *testing.T) {
	t.Parallel()

	type Server struct {
		Host  string
		Ports []int
	}
	type Config struct {
		Name    string
		Active  bool
		Score   float64
		When    time.Time
		Server  Server
		Labels  map[string]string
		Plugins []struct {
			Name    string
			Enabled bool
		}
		Any any `toml:"misc"`
	}

	input := []byte(`name = "demo"
active = true
score = 1.5
when = 2026-05-17T03:04:05Z
misc = [1, 2, 3]

[server]
host = "127.0.0.1"
ports = [80, 443]

[labels]
env = "test"

[[plugins]]
name = "cache"
enabled = true

[[plugins]]
name = "trace"
enabled = false
`)
	var got Config
	if err := Unmarshal(input, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Name != "demo" || !got.Active || got.Score != 1.5 {
		t.Fatalf("scalar fields = %#v", got)
	}
	if got.When.Format(time.RFC3339) != "2026-05-17T03:04:05Z" {
		t.Fatalf("When = %s", got.When.Format(time.RFC3339Nano))
	}
	if got.Server.Host != "127.0.0.1" || len(got.Server.Ports) != 2 || got.Server.Ports[1] != 443 {
		t.Fatalf("Server = %#v", got.Server)
	}
	if got.Labels["env"] != "test" {
		t.Fatalf("Labels = %#v", got.Labels)
	}
	if len(got.Plugins) != 2 || got.Plugins[0].Name != "cache" || got.Plugins[1].Enabled {
		t.Fatalf("Plugins = %#v", got.Plugins)
	}
	if _, ok := got.Any.([]any); !ok {
		t.Fatalf("Any = %T(%#v), want []any", got.Any, got.Any)
	}
}

func TestFacadeMarshalStructTagsOmitZeroAndRoundTrip(t *testing.T) {
	t.Parallel()

	type Server struct {
		Host string
		Port int
	}
	type Config struct {
		Name   string `toml:"name"`
		Empty  string `toml:"empty,omitzero"`
		Server Server `toml:"server"`
		Tags   []string
	}
	body, err := Marshal(Config{Name: "demo", Server: Server{Host: "localhost", Port: 8080}, Tags: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"name = \"demo\"",
		"Tags = [\"a\", \"b\"]",
		"[server]",
		"Host = \"localhost\"",
		"Port = 8080",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Marshal output missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "empty") {
		t.Fatalf("Marshal output included omitzero field\n%s", text)
	}

	var round Config
	if err := Unmarshal(body, &round); err != nil {
		t.Fatalf("roundtrip Unmarshal() error = %v\n%s", err, text)
	}
	if round.Name != "demo" || round.Server.Port != 8080 || len(round.Tags) != 2 {
		t.Fatalf("roundtrip = %#v", round)
	}
}

func TestFacadeRejectsOmitEmptyAsTypedError(t *testing.T) {
	t.Parallel()
	type Bad struct {
		Name string `toml:"name,omitempty"`
	}
	_, err := Marshal(Bad{Name: "x"})
	var tagErr *TagOptionError
	if !errors.As(err, &tagErr) || tagErr.Option != "omitempty" {
		t.Fatalf("Marshal error = %T(%v), want TagOptionError option=omitempty", err, err)
	}
}

func TestFacadeCustomHooks(t *testing.T) {
	t.Parallel()

	var encoded bytes.Buffer
	if err := NewEncoder(&encoded).Encode(customFacade{}); err != nil {
		t.Fatalf("Encode(custom) error = %v", err)
	}
	if got := encoded.String(); got != "name = \"custom\"\n" {
		t.Fatalf("encoded custom = %q", got)
	}

	var dst customFacade
	if err := Unmarshal([]byte("name = \"ignored\"\n"), &dst); err != nil {
		t.Fatalf("Unmarshal(custom) error = %v", err)
	}
	if !dst.decoded {
		t.Fatalf("custom UnmarshalTOMLFrom was not called")
	}
}

type customFacade struct{ decoded bool }

func (customFacade) MarshalTOMLTo(enc *Encoder) error {
	_, err := enc.Write([]byte("name = \"custom\"\n"))
	return err
}

func (c *customFacade) UnmarshalTOMLFrom(dec *Decoder) error {
	for {
		_, err := dec.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	c.decoded = true
	return nil
}
