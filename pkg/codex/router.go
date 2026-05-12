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

package codexappserver

import (
	"context"
	"fmt"
	"sync"
	"unsafe"

	"github.com/go-json-experiment/json"
)

const notificationQueueCapacity = 128

type turnNotificationRouter struct {
	mu      sync.Mutex
	global  chan Notification
	queues  map[string]*turnNotificationQueue
	pending map[string][]Notification
	closed  bool
	err     error
}

type turnNotificationQueue struct {
	mu            sync.Mutex
	notifications notificationRing
	notify        chan struct{}
	err           error
}

type notificationRing struct {
	values []Notification
	head   int
	tail   int
	length int
}

func newTurnNotificationRouter() *turnNotificationRouter {
	return &turnNotificationRouter{
		global:  make(chan Notification, notificationQueueCapacity),
		queues:  map[string]*turnNotificationQueue{},
		pending: map[string][]Notification{},
	}
}

func (r *turnNotificationRouter) nextGlobal(ctx context.Context, legacy <-chan Notification) (Notification, error) {
	r.mu.Lock()
	if r.closed {
		err := r.err
		r.mu.Unlock()
		if err != nil {
			return Notification{}, err
		}
		return Notification{}, &TransportClosedError{Message: "app-server notification stream closed"}
	}
	global := r.global
	r.mu.Unlock()

	select {
	case <-ctx.Done():
		return Notification{}, ctx.Err()
	case notification, ok := <-legacy:
		if !ok {
			return Notification{}, &TransportClosedError{Message: "app-server notification stream closed"}
		}
		return notification, nil
	case notification, ok := <-global:
		if !ok {
			return Notification{}, r.closedErr(&TransportClosedError{Message: "app-server notification stream closed"})
		}
		return notification, nil
	}
}

func (r *turnNotificationRouter) next(ctx context.Context, turnID string) (Notification, error) {
	r.mu.Lock()
	if r.closed {
		err := r.err
		r.mu.Unlock()
		return Notification{}, err
	}
	queue := r.queues[turnID]
	r.mu.Unlock()
	if queue == nil {
		return Notification{}, fmt.Errorf("turn consumer is not active for %s", turnID)
	}
	return queue.next(ctx)
}

func (r *turnNotificationRouter) register(turnID string) (*turnNotificationQueue, error) {
	if turnID == "" {
		return nil, fmt.Errorf("turn notification router: empty turn id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, r.err
	}
	if _, ok := r.queues[turnID]; ok {
		return nil, fmt.Errorf("turn consumer already active for %s", turnID)
	}
	queue := &turnNotificationQueue{
		notifications: newNotificationRing(notificationQueueCapacity),
		notify:        make(chan struct{}, 1),
	}
	if pending := r.pending[turnID]; len(pending) > 0 {
		if !queue.notifications.appendAll(pending) {
			return nil, fmt.Errorf("turn notification router: pending queue overflow for %s", turnID)
		}
		delete(r.pending, turnID)
	}
	r.queues[turnID] = queue
	return queue, nil
}

func (r *turnNotificationRouter) queue(turnID string) (*turnNotificationQueue, error) {
	if turnID == "" {
		return nil, fmt.Errorf("turn notification router: empty turn id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, r.err
	}
	queue := r.queues[turnID]
	if queue == nil {
		return nil, fmt.Errorf("turn notification router: no active consumer for %s", turnID)
	}
	return queue, nil
}

func (r *turnNotificationRouter) unregister(turnID string) {
	if turnID == "" {
		return
	}
	r.mu.Lock()
	delete(r.queues, turnID)
	r.mu.Unlock()
}

func (r *turnNotificationRouter) clearPending(turnID string) {
	if turnID == "" {
		return
	}
	r.mu.Lock()
	delete(r.pending, turnID)
	r.mu.Unlock()
}

func (r *turnNotificationRouter) route(notification Notification) error {
	turnID := notificationTurnID(notification)

	r.mu.Lock()
	if r.closed {
		err := r.err
		r.mu.Unlock()
		return err
	}
	if turnID == "" {
		select {
		case r.global <- notification:
			r.mu.Unlock()
			return nil
		default:
		}
		err := fmt.Errorf("notification router: global notification queue full")
		r.failLocked(err)
		r.mu.Unlock()
		return err
	}
	if queue := r.queues[turnID]; queue != nil {
		if err := queue.push(notification); err != nil {
			r.failLocked(err)
			r.mu.Unlock()
			return err
		}
		r.mu.Unlock()
		return nil
	}
	pending := r.pending[turnID]
	if len(pending) >= notificationQueueCapacity {
		err := fmt.Errorf("notification router: pending queue full for %s", turnID)
		r.failLocked(err)
		r.mu.Unlock()
		return err
	}
	r.pending[turnID] = append(pending, notification)
	r.mu.Unlock()
	return nil
}

func (r *turnNotificationRouter) close(err error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.err = err
	global := r.global
	r.global = nil
	queues := make([]*turnNotificationQueue, 0, len(r.queues))
	for _, queue := range r.queues {
		queues = append(queues, queue)
	}
	r.queues = map[string]*turnNotificationQueue{}
	r.pending = map[string][]Notification{}
	r.mu.Unlock()

	if global != nil {
		close(global)
	}
	for _, queue := range queues {
		queue.close(err)
	}
}

func (r *turnNotificationRouter) failLocked(err error) {
	if r.closed {
		return
	}
	r.closed = true
	r.err = err
	global := r.global
	r.global = nil
	queues := make([]*turnNotificationQueue, 0, len(r.queues))
	for _, queue := range r.queues {
		queues = append(queues, queue)
	}
	r.queues = map[string]*turnNotificationQueue{}
	r.pending = map[string][]Notification{}
	if global != nil {
		close(global)
	}
	for _, queue := range queues {
		queue.close(err)
	}
}

func (r *turnNotificationRouter) closedErr(err error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	return err
}

func (q *turnNotificationQueue) push(notification Notification) error {
	q.mu.Lock()
	if q.err != nil {
		err := q.err
		q.mu.Unlock()
		return err
	}
	if !q.notifications.push(notification) {
		q.mu.Unlock()
		return fmt.Errorf("turn notification queue full")
	}
	select {
	case q.notify <- struct{}{}:
	default:
	}
	q.mu.Unlock()
	return nil
}

func (q *turnNotificationQueue) pop() (Notification, bool) {
	q.mu.Lock()
	notification, ok := q.notifications.pop()
	q.mu.Unlock()
	return notification, ok
}

func (q *turnNotificationQueue) next(ctx context.Context) (Notification, error) {
	for {
		q.mu.Lock()
		if notification, ok := q.notifications.pop(); ok {
			q.mu.Unlock()
			return notification, nil
		}
		if q.err != nil {
			err := q.err
			q.mu.Unlock()
			return Notification{}, err
		}
		notify := q.notify
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return Notification{}, ctx.Err()
		case <-notify:
		}
	}
}

func (q *turnNotificationQueue) close(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return
	}
	q.err = err
	select {
	case q.notify <- struct{}{}:
	default:
	}
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

func notificationTurnID(notification Notification) string {
	if turnID, ok := scanNotificationTurnID(notification.Params); ok {
		return turnID
	}
	return decodeNotificationTurnID(notification)
}

type scannedTurn struct {
	id      string
	turnID  string
	turnID2 string
}

type notificationJSONScanner struct {
	data []byte
	pos  int
}

func scanNotificationTurnID(params []byte) (string, bool) {
	scanner := notificationJSONScanner{data: params}
	scanner.skipSpace()
	if scanner.consumeLiteral("null") {
		scanner.skipSpace()
		return "", scanner.done()
	}
	if !scanner.consumeByte('{') {
		return "", false
	}

	var turnID, turnID2 string
	var turn scannedTurn
	for {
		scanner.skipSpace()
		if scanner.consumeByte('}') {
			break
		}
		key, escaped, ok := scanner.readString()
		if !ok || escaped {
			return "", false
		}
		scanner.skipSpace()
		if !scanner.consumeByte(':') {
			return "", false
		}
		switch {
		case bytesEqualString(key, "turnId"):
			value, ok := scanner.readSimpleString()
			if !ok {
				return "", false
			}
			turnID = value
		case bytesEqualString(key, "turn_id"):
			value, ok := scanner.readSimpleString()
			if !ok {
				return "", false
			}
			turnID2 = value
		case bytesEqualString(key, "turn"):
			value, ok := scanner.readTurnObject()
			if !ok {
				return "", false
			}
			turn = value
		default:
			if !scanner.skipValue() {
				return "", false
			}
		}
		scanner.skipSpace()
		if scanner.consumeByte('}') {
			break
		}
		if !scanner.consumeByte(',') {
			return "", false
		}
	}
	scanner.skipSpace()
	if !scanner.done() {
		return "", false
	}
	if turnID != "" {
		return turnID, true
	}
	if turnID2 != "" {
		return turnID2, true
	}
	if turn.turnID != "" {
		return turn.turnID, true
	}
	if turn.turnID2 != "" {
		return turn.turnID2, true
	}
	return turn.id, true
}

func (s *notificationJSONScanner) readTurnObject() (scannedTurn, bool) {
	s.skipSpace()
	if s.consumeLiteral("null") {
		return scannedTurn{}, true
	}
	if !s.consumeByte('{') {
		return scannedTurn{}, false
	}

	var turn scannedTurn
	for {
		s.skipSpace()
		if s.consumeByte('}') {
			return turn, true
		}
		key, escaped, ok := s.readString()
		if !ok || escaped {
			return scannedTurn{}, false
		}
		s.skipSpace()
		if !s.consumeByte(':') {
			return scannedTurn{}, false
		}
		switch {
		case bytesEqualString(key, "id"):
			value, ok := s.readSimpleString()
			if !ok {
				return scannedTurn{}, false
			}
			turn.id = value
		case bytesEqualString(key, "turnId"):
			value, ok := s.readSimpleString()
			if !ok {
				return scannedTurn{}, false
			}
			turn.turnID = value
		case bytesEqualString(key, "turn_id"):
			value, ok := s.readSimpleString()
			if !ok {
				return scannedTurn{}, false
			}
			turn.turnID2 = value
		default:
			if !s.skipValue() {
				return scannedTurn{}, false
			}
		}
		s.skipSpace()
		if s.consumeByte('}') {
			return turn, true
		}
		if !s.consumeByte(',') {
			return scannedTurn{}, false
		}
	}
}

func (s *notificationJSONScanner) skipValue() bool {
	s.skipSpace()
	if s.done() {
		return false
	}
	switch s.data[s.pos] {
	case '"':
		_, _, ok := s.readString()
		return ok
	case '{':
		return s.skipObject()
	case '[':
		return s.skipArray()
	case 't':
		return s.consumeLiteral("true")
	case 'f':
		return s.consumeLiteral("false")
	case 'n':
		return s.consumeLiteral("null")
	default:
		return s.skipNumber()
	}
}

func (s *notificationJSONScanner) skipObject() bool {
	if !s.consumeByte('{') {
		return false
	}
	for {
		s.skipSpace()
		if s.consumeByte('}') {
			return true
		}
		if _, _, ok := s.readString(); !ok {
			return false
		}
		s.skipSpace()
		if !s.consumeByte(':') {
			return false
		}
		if !s.skipValue() {
			return false
		}
		s.skipSpace()
		if s.consumeByte('}') {
			return true
		}
		if !s.consumeByte(',') {
			return false
		}
	}
}

func (s *notificationJSONScanner) skipArray() bool {
	if !s.consumeByte('[') {
		return false
	}
	for {
		s.skipSpace()
		if s.consumeByte(']') {
			return true
		}
		if !s.skipValue() {
			return false
		}
		s.skipSpace()
		if s.consumeByte(']') {
			return true
		}
		if !s.consumeByte(',') {
			return false
		}
	}
}

func (s *notificationJSONScanner) skipNumber() bool {
	start := s.pos
	if s.pos < len(s.data) && s.data[s.pos] == '-' {
		s.pos++
	}
	if s.pos >= len(s.data) {
		return false
	}
	if s.data[s.pos] == '0' {
		s.pos++
	} else if isDigitOneToNine(s.data[s.pos]) {
		s.pos++
		for s.pos < len(s.data) && isDigit(s.data[s.pos]) {
			s.pos++
		}
	} else {
		return false
	}
	if s.pos < len(s.data) && s.data[s.pos] == '.' {
		s.pos++
		if s.pos >= len(s.data) || !isDigit(s.data[s.pos]) {
			return false
		}
		for s.pos < len(s.data) && isDigit(s.data[s.pos]) {
			s.pos++
		}
	}
	if s.pos < len(s.data) && (s.data[s.pos] == 'e' || s.data[s.pos] == 'E') {
		s.pos++
		if s.pos < len(s.data) && (s.data[s.pos] == '+' || s.data[s.pos] == '-') {
			s.pos++
		}
		if s.pos >= len(s.data) || !isDigit(s.data[s.pos]) {
			return false
		}
		for s.pos < len(s.data) && isDigit(s.data[s.pos]) {
			s.pos++
		}
	}
	return s.pos > start
}

func (s *notificationJSONScanner) readSimpleString() (string, bool) {
	value, escaped, ok := s.readString()
	if !ok || escaped {
		return "", false
	}
	if len(value) == 0 {
		return "", true
	}
	// SAFETY: value aliases Notification.Params. The returned string is used for
	// immediate map lookups or as a pending map key while the corresponding
	// Notification is stored in the same pending queue, keeping Params alive.
	return unsafe.String(unsafe.SliceData(value), len(value)), true
}

func (s *notificationJSONScanner) readString() ([]byte, bool, bool) {
	if !s.consumeByte('"') {
		return nil, false, false
	}
	start := s.pos
	escaped := false
	for s.pos < len(s.data) {
		switch c := s.data[s.pos]; c {
		case '"':
			value := s.data[start:s.pos]
			s.pos++
			return value, escaped, true
		case '\\':
			escaped = true
			s.pos++
			if s.pos >= len(s.data) {
				return nil, false, false
			}
			switch s.data[s.pos] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				s.pos++
			case 'u':
				s.pos++
				for range 4 {
					if s.pos >= len(s.data) || !isHexDigit(s.data[s.pos]) {
						return nil, false, false
					}
					s.pos++
				}
			default:
				return nil, false, false
			}
		default:
			if c < 0x20 {
				return nil, false, false
			}
			s.pos++
		}
	}
	return nil, false, false
}

func (s *notificationJSONScanner) skipSpace() {
	for s.pos < len(s.data) {
		switch s.data[s.pos] {
		case ' ', '\n', '\r', '\t':
			s.pos++
		default:
			return
		}
	}
}

func (s *notificationJSONScanner) consumeByte(c byte) bool {
	if s.pos >= len(s.data) || s.data[s.pos] != c {
		return false
	}
	s.pos++
	return true
}

func (s *notificationJSONScanner) consumeLiteral(literal string) bool {
	if len(s.data)-s.pos < len(literal) {
		return false
	}
	for i := range len(literal) {
		if s.data[s.pos+i] != literal[i] {
			return false
		}
	}
	s.pos += len(literal)
	return true
}

func (s *notificationJSONScanner) done() bool {
	return s.pos == len(s.data)
}

func bytesEqualString(value []byte, want string) bool {
	if len(value) != len(want) {
		return false
	}
	for i := range value {
		if value[i] != want[i] {
			return false
		}
	}
	return true
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isDigitOneToNine(c byte) bool {
	return c >= '1' && c <= '9'
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func decodeNotificationTurnID(notification Notification) string {
	var envelope struct {
		TurnID  string `json:"turnId"`
		TurnID2 string `json:"turn_id"`
		Turn    *struct {
			ID      string `json:"id"`
			TurnID  string `json:"turnId"`
			TurnID2 string `json:"turn_id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(notification.Params, &envelope); err != nil {
		return ""
	}
	if envelope.TurnID != "" {
		return envelope.TurnID
	}
	if envelope.TurnID2 != "" {
		return envelope.TurnID2
	}
	if envelope.Turn != nil {
		if envelope.Turn.TurnID != "" {
			return envelope.Turn.TurnID
		}
		if envelope.Turn.TurnID2 != "" {
			return envelope.Turn.TurnID2
		}
		return envelope.Turn.ID
	}
	return ""
}
