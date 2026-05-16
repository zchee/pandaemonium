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

package reflectcache

import (
	"errors"
	"reflect"
	"testing"
)

func TestLookupCachesMetadataAndRejectsTypedOmitempty(t *testing.T) {
	t.Parallel()

	type sample struct {
		Name  string
		Zero  string `toml:"zero,omitzero"`
		Skip  string `toml:"-"`
		Bad   string `toml:"bad,omitempty"`
		inner string
	}

	info, err := Lookup(reflect.TypeOf(sample{}))
	if err == nil {
		t.Fatalf("Lookup() error = nil, want invalid tag option error")
	}
	var tagErr *InvalidTagOptionError
	if !errors.As(err, &tagErr) || tagErr.Option != "omitempty" {
		t.Fatalf("Lookup() error = %T(%v), want InvalidTagOptionError option=omitempty", err, err)
	}
	if info != nil {
		t.Fatalf("Lookup() info = %#v, want nil on error", info)
	}
}

func TestLookupCachesFieldMetadata(t *testing.T) {
	t.Parallel()

	type sample struct {
		Name string
		Zero string `toml:"zero,omitzero"`
		Skip string `toml:"-"`
	}

	info, err := Lookup(reflect.TypeOf(sample{}))
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if info.Type != reflect.TypeOf(sample{}) {
		t.Fatalf("Type = %v, want %v", info.Type, reflect.TypeOf(sample{}))
	}
	if got, ok := info.ByName["Name"]; !ok || got.Name != "Name" || got.OmitZero {
		t.Fatalf("ByName[Name] = %#v, %v", got, ok)
	}
	if got, ok := info.ByName["name"]; !ok || got.Name != "Name" || got.OmitZero {
		t.Fatalf("ByName[name] = %#v, %v", got, ok)
	}
	if got, ok := info.ByName["zero"]; !ok || !got.OmitZero || got.Name != "zero" {
		t.Fatalf("ByName[zero] = %#v, %v", got, ok)
	}
	if _, ok := info.ByName["Skip"]; ok {
		t.Fatalf("Skip should not be indexed: %#v", info.ByName)
	}
	if len(info.Fields) != 2 {
		t.Fatalf("Fields len = %d, want 2", len(info.Fields))
	}

	again, err := Lookup(reflect.TypeOf(sample{}))
	if err != nil {
		t.Fatalf("Lookup() cached error = %v", err)
	}
	if again != info {
		t.Fatalf("Lookup() cache miss: %p != %p", again, info)
	}
}
