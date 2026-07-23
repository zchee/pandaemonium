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
	"testing"
	"time"
)

func TestWaitUntil(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		done     func() <-chan error
		deadline func() time.Time
		want     bool
	}{
		"success: nil channel returns false immediately": {
			done:     func() <-chan error { return nil },
			deadline: func() time.Time { return time.Now().Add(time.Hour) },
			want:     false,
		},
		"success: ready value with expired deadline still polls": {
			done: func() <-chan error {
				done := make(chan error, 1)
				done <- nil
				return done
			},
			deadline: func() time.Time { return time.Now().Add(-time.Second) },
			want:     true,
		},
		"success: empty channel with expired deadline returns false": {
			done:     func() <-chan error { return make(chan error, 1) },
			deadline: func() time.Time { return time.Now().Add(-time.Second) },
			want:     false,
		},
		"success: value before deadline returns true": {
			done: func() <-chan error {
				done := make(chan error, 1)
				go func() {
					time.Sleep(10 * time.Millisecond)
					done <- nil
				}()
				return done
			},
			deadline: func() time.Time { return time.Now().Add(2 * time.Second) },
			want:     true,
		},
		"success: deadline before value returns false": {
			done:     func() <-chan error { return make(chan error, 1) },
			deadline: func() time.Time { return time.Now().Add(20 * time.Millisecond) },
			want:     false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := WaitUntil(tt.done(), tt.deadline()); got != tt.want {
				t.Errorf("WaitUntil() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestWaitForCommand(t *testing.T) {
	t.Parallel()

	cmd := exec.CommandContext(t.Context(), "true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	done := WaitForCommand(cmd)
	if err := <-done; err != nil {
		t.Fatalf("wait error = %v, want nil", err)
	}
	// The channel is closed after the send: later receives complete too.
	if _, ok := <-done; ok {
		t.Error("second receive returned an unexpected value; channel should be closed")
	}
}

func TestTerminateCommandGraceful(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	done := WaitForCommand(cmd)
	start := time.Now()
	TerminateCommand(cmd, done, os.Interrupt, time.Now().Add(5*time.Second), time.Time{})
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("graceful terminate took %v; process ignored the signal path", elapsed)
	}
	if cmd.ProcessState == nil {
		t.Fatal("process not reaped after TerminateCommand")
	}
}

func TestTerminateCommandEscalatesToKill(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sh", "-c", `trap "" INT TERM; sleep 30`)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	done := WaitForCommand(cmd)
	start := time.Now()
	TerminateCommand(cmd, done, os.Interrupt, time.Now().Add(150*time.Millisecond), time.Time{})
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("kill escalation took %v; zero killDeadline should reap promptly after Kill", elapsed)
	}
	if cmd.ProcessState == nil {
		t.Fatal("process not reaped after kill escalation")
	}
}
