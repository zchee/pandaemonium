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

package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/zchee/pandaemonium/pkg/codex/examples/internal/exampleutil"
)

func main() {
	ctx, cancel := exampleutil.NewContext()
	defer cancel()

	client, err := exampleutil.NewCodex(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("server:", exampleutil.ServerLabel(client.Metadata()))
	models, err := client.Models(ctx, false)
	if err != nil {
		log.Fatal(err)
	}
	ids := make([]string, 0, min(5, len(models.Data)))
	for _, model := range models.Data[:min(5, len(models.Data))] {
		ids = append(ids, model.ID)
	}
	if len(ids) == 0 {
		ids = append(ids, "[none]")
	}
	fmt.Println("models.count:", len(models.Data))
	fmt.Println("models:", strings.Join(ids, ", "))
}
