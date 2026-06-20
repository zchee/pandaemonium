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
	"os/signal"

	"golang.org/x/sys/unix"

	"github.com/zchee/pandaemonium/cmd/agu/cmd"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), unix.SIGINT, unix.SIGTERM)
	defer cancel()

	if err := cmd.Execute(ctx); err != nil {
		if exitErr, ok := errors.AsType[cmd.ExitError](err); ok {
			fmt.Fprintln(os.Stderr, exitErr)
			return exitErr.Code()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
