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

package claude

// TaskBudget is an API-side task budget in tokens. When set on [Options], the
// model is made aware of its remaining token budget so it can pace tool use
// and wrap up before the limit. The CLI forwards the budget as
// output_config.task_budget under the task-budgets-2026-03-13 beta header.
//
// Mirrors upstream TaskBudget (types.py:63); kept as a struct (rather than a
// bare int on Options) because upstream uses a TypedDict so future budget
// fields can be added without an API break.
type TaskBudget struct {
	// Total is the total token budget for the task. Sent as the sole value of
	// --task-budget at launch; the zero value is wire-valid and is forwarded
	// (parity with upstream's `is not None` gate at subprocess_cli.py:268).
	Total int
}
