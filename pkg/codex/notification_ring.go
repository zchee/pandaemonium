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

package codex

type notificationRing struct {
	values []Notification
	head   int
	tail   int
	length int
}

func newNotificationRing(capacity int) notificationRing {
	return notificationRing{values: make([]Notification, capacity)}
}

func (r *notificationRing) len() int {
	if r == nil {
		return 0
	}
	return r.length
}

func (r *notificationRing) push(notification Notification) bool {
	if r == nil || len(r.values) == 0 || r.length >= len(r.values) {
		return false
	}
	r.values[r.tail] = notification
	r.tail++
	if r.tail == len(r.values) {
		r.tail = 0
	}
	r.length++
	return true
}

func (r *notificationRing) appendAll(notifications []Notification) bool {
	if r == nil || len(notifications) > len(r.values)-r.length {
		return false
	}
	if len(notifications) == 0 {
		return true
	}
	copied := copy(r.values[r.tail:], notifications)
	if copied < len(notifications) {
		copy(r.values, notifications[copied:])
	}
	r.tail = (r.tail + len(notifications)) % len(r.values)
	r.length += len(notifications)
	return true
}

func (r *notificationRing) pop() (Notification, bool) {
	if r == nil || r.length == 0 {
		return Notification{}, false
	}
	notification := r.values[r.head]
	r.values[r.head] = Notification{}
	r.head++
	if r.head == len(r.values) {
		r.head = 0
	}
	r.length--
	if r.length == 0 {
		r.head = 0
		r.tail = 0
	}
	return notification, true
}
