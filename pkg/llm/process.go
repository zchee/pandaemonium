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

package llm

import (
	"os"
	"os/exec"
	"time"
)

// WaitForCommand starts a goroutine that calls cmd.Wait and returns a
// 1-buffered channel that receives the exit error and is then closed, so any
// number of later receives observe completion.
func WaitForCommand(cmd *exec.Cmd) chan error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		close(done)
	}()
	return done
}

// WaitUntil waits for a value on done until deadline, reporting whether one
// arrived. A nil channel reports false immediately; an expired deadline still
// polls done once without blocking.
func WaitUntil[T any](done <-chan T, deadline time.Time) bool {
	if done == nil {
		return false
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// TerminateCommand signals cmd with sig, waits until sigDeadline for it to
// exit, and escalates to Kill, waiting until killDeadline for the reap. A
// zero killDeadline waits indefinitely for the post-kill reap. A nil done
// falls back to [WaitForCommand]; callers that already reap the process pass
// their own channel.
func TerminateCommand(cmd *exec.Cmd, done <-chan error, sig os.Signal, sigDeadline, killDeadline time.Time) {
	if cmd == nil {
		return
	}
	if cmd.Process != nil {
		_ = cmd.Process.Signal(sig)
	}
	if done == nil {
		done = WaitForCommand(cmd)
	}
	if WaitUntil(done, sigDeadline) {
		return
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if killDeadline.IsZero() {
		<-done
		return
	}
	_ = WaitUntil(done, killDeadline)
}
