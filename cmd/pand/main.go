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
	"context"
	"errors"
	"fmt"
	"os"

	pandcmd "github.com/zchee/pandaemonium/cmd/pand/internal/cmd"
)

func main() {
	if err := pandcmd.Execute(context.Background()); err != nil {
		if exitErr, ok := errors.AsType[pandcmd.ExitError](err); ok {
			os.Exit(exitErr.Code())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
