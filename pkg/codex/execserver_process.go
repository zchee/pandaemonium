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

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

const (
	// ExecServerInitializeMethod is the method name for initializing the exec-server.
	ExecServerInitializeMethod = "initialize"
	// ExecServerInitializedMethod is the method name sent after initialization is complete.
	ExecServerInitializedMethod = "initialized"
	// ExecServerProcessStartMethod is the method name for starting a process.
	ExecServerProcessStartMethod = "process/start"
	// ExecServerProcessReadMethod is the method name for reading process output.
	ExecServerProcessReadMethod = "process/read"
	// ExecServerProcessWriteMethod is the method name for writing to a process's stdin.
	ExecServerProcessWriteMethod = "process/write"
	// ExecServerProcessTerminateMethod is the method name for terminating a process.
	ExecServerProcessTerminateMethod = "process/terminate"
	// ExecServerProcessOutputMethod is the notification method for streaming process output.
	ExecServerProcessOutputMethod = "process/output"
	// ExecServerProcessExitedMethod is the notification method for process exit events.
	ExecServerProcessExitedMethod = "process/exited"
	// ExecServerProcessClosedMethod is the notification method for process close events.
	ExecServerProcessClosedMethod = "process/closed"

	execServerTransportClosedMessage    = "exec-server is not running"
	execServerTransportClosedEOFMessage = "exec-server closed stdout"
)

const execServerProcessNotificationDecodeForm = "decode %s notification: %w"

var (
	errExecServerUnexpectedServerRequest  = errors.New("unexpected server request")
	errExecServerProcessNotificationNoID  = errors.New("missing processId")
	errExecServerProcessNotificationNoSeq = errors.New("missing seq")
	errProcessHandleNil                   = errors.New("process handle is nil")
)

// ByteChunk serializes bytes as a base64 string.
type ByteChunk []byte

var (
	_ json.MarshalerTo     = ByteChunk{}
	_ json.UnmarshalerFrom = (*ByteChunk)(nil)
)

// MarshalJSONTo implements [json.MarshalerTo].
func (b ByteChunk) MarshalJSONTo(enc *jsontext.Encoder) error {
	err := enc.WriteToken(jsontext.String(base64.StdEncoding.EncodeToString(b)))
	if err != nil {
		return fmt.Errorf("marshal byte chunk: %w", err)
	}

	return nil
}

// UnmarshalJSONFrom implements [json.UnmarshalerFrom].
func (b *ByteChunk) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var encoded string
	err := json.UnmarshalDecode(dec, &encoded)
	if err != nil {
		return fmt.Errorf("unmarshal byte chunk string: %w", err)
	}
	if encoded == "" {
		*b = nil

		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("unmarshal byte chunk base64: %w", err)
	}
	*b = decoded
	return nil
}

// ProcessID identifies a logical process handle in the exec-server protocol.
type ProcessID string

// ExecServerInitializeParams starts the local exec-server handshake.
type ExecServerInitializeParams struct {
	ClientName      string  `json:"clientName"`
	ResumeSessionID *string `json:"resumeSessionId,omitzero"`
}

// ExecServerInitializeResponse returns the session id assigned by the server.
type ExecServerInitializeResponse struct {
	SessionID string `json:"sessionId"`
}

// ExecServerEnvPolicy mirrors the exec-server shell environment policy payload.
type ExecServerEnvPolicy struct {
	Inherit               string            `json:"inherit"`
	IgnoreDefaultExcludes bool              `json:"ignoreDefaultExcludes"`
	Exclude               []string          `json:"exclude"`
	Set                   map[string]string `json:"set"`
	IncludeOnly           []string          `json:"includeOnly"`
}

// ExecServerProcessStartParams starts a new process.
type ExecServerProcessStartParams struct {
	ProcessID ProcessID            `json:"processId"`
	Argv      []string             `json:"argv"`
	Cwd       string               `json:"cwd"`
	EnvPolicy *ExecServerEnvPolicy `json:"envPolicy,omitzero"`
	Env       map[string]string    `json:"env"`
	TTY       bool                 `json:"tty"`
	PipeStdin bool                 `json:"pipeStdin,omitzero"`
	Arg0      *string              `json:"arg0,omitzero"`
}

// ExecServerProcessStartResponse returns the assigned process id.
type ExecServerProcessStartResponse struct {
	ProcessID ProcessID `json:"processId"`
}

// ExecServerProcessReadParams requests retained output for a process.
type ExecServerProcessReadParams struct {
	ProcessID ProcessID `json:"processId"`
	AfterSeq  *uint64   `json:"afterSeq,omitzero"`
	MaxBytes  *uint64   `json:"maxBytes,omitzero"`
	WaitMs    *uint64   `json:"waitMs,omitzero"`
}

// ExecServerProcessOutputStream labels an output stream in the process API.
type ExecServerProcessOutputStream string

const (
	// ExecServerProcessOutputStreamStdout represents the standard output stream.
	ExecServerProcessOutputStreamStdout ExecServerProcessOutputStream = "stdout"
	// ExecServerProcessOutputStreamStderr represents the standard error stream.
	ExecServerProcessOutputStreamStderr ExecServerProcessOutputStream = "stderr"
	// ExecServerProcessOutputStreamPty represents the PTY stream.
	ExecServerProcessOutputStreamPty ExecServerProcessOutputStream = "pty"
)

// ExecServerProcessOutputChunk is a retained-output chunk returned by process/read.
type ExecServerProcessOutputChunk struct {
	Seq    uint64                        `json:"seq"`
	Stream ExecServerProcessOutputStream `json:"stream"`
	Chunk  ByteChunk                     `json:"chunk"`
}

// ExecServerProcessReadResponse returns retained output and process state.
type ExecServerProcessReadResponse struct {
	Chunks   []ExecServerProcessOutputChunk `json:"chunks"`
	NextSeq  uint64                         `json:"nextSeq"`
	Exited   bool                           `json:"exited"`
	ExitCode *int32                         `json:"exitCode,omitzero"`
	Closed   bool                           `json:"closed"`
	Failure  *string                        `json:"failure,omitzero"`
}

// ExecServerProcessWriteParams writes stdin bytes to a process.
type ExecServerProcessWriteParams struct {
	ProcessID ProcessID `json:"processId"`
	Chunk     ByteChunk `json:"chunk"`
}

// ExecServerProcessWriteStatus reports the result of process/write.
type ExecServerProcessWriteStatus string

const (
	// ExecServerProcessWriteStatusAccepted indicates the write was accepted.
	ExecServerProcessWriteStatusAccepted ExecServerProcessWriteStatus = "accepted"
	// ExecServerProcessWriteStatusUnknownProcess indicates the process is unknown.
	ExecServerProcessWriteStatusUnknownProcess ExecServerProcessWriteStatus = "unknownProcess"
	// ExecServerProcessWriteStatusStdinClosed indicates the stdin stream is closed.
	ExecServerProcessWriteStatusStdinClosed ExecServerProcessWriteStatus = "stdinClosed"
	// ExecServerProcessWriteStatusStarting indicates the process is still starting.
	ExecServerProcessWriteStatusStarting ExecServerProcessWriteStatus = "starting"
)

// ExecServerProcessWriteResponse reports whether the write was accepted.
type ExecServerProcessWriteResponse struct {
	Status ExecServerProcessWriteStatus `json:"status"`
}

// ExecServerProcessTerminateParams asks the server to terminate a process.
type ExecServerProcessTerminateParams struct {
	ProcessID ProcessID `json:"processId"`
}

// ExecServerProcessTerminateResponse reports whether the process is still running.
type ExecServerProcessTerminateResponse struct {
	Running bool `json:"running"`
}

// ExecServerProcessNotification is the shared interface for ordered process events.
type ExecServerProcessNotification interface {
	isExecServerProcessNotification()
	ProcessIDValue() ProcessID
	SeqValue() uint64
}

// ExecServerProcessOutputNotification streams process output.
type ExecServerProcessOutputNotification struct {
	ProcessID ProcessID                     `json:"processId"`
	Seq       uint64                        `json:"seq"`
	Stream    ExecServerProcessOutputStream `json:"stream"`
	Chunk     ByteChunk                     `json:"chunk"`
}

func (ExecServerProcessOutputNotification) isExecServerProcessNotification() {}

// ProcessIDValue implements [ExecServerProcessNotification].
func (n ExecServerProcessOutputNotification) ProcessIDValue() ProcessID { return n.ProcessID }

// SeqValue implements [ExecServerProcessNotification].
func (n ExecServerProcessOutputNotification) SeqValue() uint64 { return n.Seq }

// ExecServerProcessExitedNotification marks a process exit.
type ExecServerProcessExitedNotification struct {
	ProcessID ProcessID `json:"processId"`
	Seq       uint64    `json:"seq"`
	ExitCode  int32     `json:"exitCode"`
}

func (ExecServerProcessExitedNotification) isExecServerProcessNotification() {}

// ProcessIDValue implements [ExecServerProcessNotification].
func (n ExecServerProcessExitedNotification) ProcessIDValue() ProcessID { return n.ProcessID }

// SeqValue implements [ExecServerProcessNotification].
func (n ExecServerProcessExitedNotification) SeqValue() uint64 { return n.Seq }

// ExecServerProcessClosedNotification marks a process handle as closed.
type ExecServerProcessClosedNotification struct {
	ProcessID ProcessID `json:"processId"`
	Seq       uint64    `json:"seq"`
}

func (ExecServerProcessClosedNotification) isExecServerProcessNotification() {}

// ProcessIDValue implements [ExecServerProcessNotification].
func (n ExecServerProcessClosedNotification) ProcessIDValue() ProcessID { return n.ProcessID }

// SeqValue implements [ExecServerProcessNotification].
func (n ExecServerProcessClosedNotification) SeqValue() uint64 { return n.Seq }

// ExecServerClient is a lightweight JSON-RPC client for the local exec-server protocol.
type ExecServerClient struct {
	transport atomic.Pointer[Transport]

	closeMu  sync.Mutex
	rpcState *jsonRPCClientState

	processMu     sync.Mutex
	processQueues map[ProcessID]*execServerProcessQueue
	readDone      chan struct{}
}

// NewExecServerClient constructs a client around an existing transport.
func NewExecServerClient(transport Transport) *ExecServerClient {
	client := &ExecServerClient{
		rpcState:      newJSONRPCClientState(),
		processQueues: map[ProcessID]*execServerProcessQueue{},
		readDone:      make(chan struct{}),
	}
	if transport == nil {
		close(client.readDone)
		return client
	}
	client.storeTransport(transport)
	go client.readLoop(context.Background(), transport, client.readDone)
	return client
}

// Close closes the underlying transport and waits for the read loop to finish.
func (c *ExecServerClient) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	closedErr := &TransportClosedError{Message: execServerTransportClosedMessage}
	c.failPending(closedErr)
	c.closeAllProcessQueues(closedErr)

	c.rpcState.lockWrite()
	t := c.loadTransport()
	c.storeTransport(nil)
	var closeErr error
	if t != nil {
		closeErr = t.Close()
	}
	c.rpcState.unlockWrite()

	<-c.readDone
	return closeErr
}

// Initialize sends initialize and then initialized.
func (c *ExecServerClient) Initialize(ctx context.Context, params *ExecServerInitializeParams) (ExecServerInitializeResponse, error) {
	raw, err := c.RequestRaw(ctx, ExecServerInitializeMethod, paramsOrEmpty(params))
	if err != nil {
		return ExecServerInitializeResponse{}, err
	}
	var response ExecServerInitializeResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return ExecServerInitializeResponse{}, fmt.Errorf("decode %s response: %w", ExecServerInitializeMethod, err)
	}
	if err := c.Notify(ctx, ExecServerInitializedMethod, nil); err != nil {
		return ExecServerInitializeResponse{}, err
	}
	return response, nil
}

// ProcessStart starts a process and returns a handle for follow-up calls.
func (c *ExecServerClient) ProcessStart(ctx context.Context, params *ExecServerProcessStartParams) (*ExecServerProcessHandle, error) {
	raw, err := c.RequestRaw(ctx, ExecServerProcessStartMethod, paramsOrEmpty(params))
	if err != nil {
		return nil, err
	}
	var response ExecServerProcessStartResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", ExecServerProcessStartMethod, err)
	}
	queue := c.ensureProcessQueue(response.ProcessID)
	handle := &ExecServerProcessHandle{client: c, processID: response.ProcessID, processQueue: queue}
	return handle, nil
}

// ProcessRead reads retained output for a process.
func (c *ExecServerClient) ProcessRead(ctx context.Context, params *ExecServerProcessReadParams) (ExecServerProcessReadResponse, error) {
	return request[ExecServerProcessReadResponse](ctx, c, ExecServerProcessReadMethod, paramsOrEmpty(params))
}

// ProcessWrite writes stdin bytes to a process.
func (c *ExecServerClient) ProcessWrite(ctx context.Context, params *ExecServerProcessWriteParams) (ExecServerProcessWriteResponse, error) {
	return request[ExecServerProcessWriteResponse](ctx, c, ExecServerProcessWriteMethod, paramsOrEmpty(params))
}

// ProcessTerminate terminates a running process.
func (c *ExecServerClient) ProcessTerminate(ctx context.Context, params *ExecServerProcessTerminateParams) (ExecServerProcessTerminateResponse, error) {
	return request[ExecServerProcessTerminateResponse](ctx, c, ExecServerProcessTerminateMethod, paramsOrEmpty(params))
}

// RequestRaw sends a request and returns the raw result JSON.
func (c *ExecServerClient) RequestRaw(ctx context.Context, method string, params any) (jsontext.Value, error) {
	return c.rpcState.requestRaw(ctx, method, params, c.writeMessage)
}

// Notify sends a JSON-RPC notification to the exec-server.
func (c *ExecServerClient) Notify(ctx context.Context, method string, params any) error {
	return c.rpcState.notify(ctx, method, params, c.writeMessage)
}

// NextProcessNotification waits for the next ordered process event.
func (c *ExecServerClient) NextProcessNotification(ctx context.Context, processID ProcessID) (ExecServerProcessNotification, error) {
	queue := c.ensureProcessQueue(processID)
	return c.nextProcessNotification(ctx, processID, queue)
}

func request[T any](ctx context.Context, c *ExecServerClient, method string, params any) (T, error) {
	var zero T
	raw, err := c.RequestRaw(ctx, method, params)
	if err != nil {
		return zero, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return zero, nil
	}
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return zero, fmt.Errorf("decode %s response: %w", method, err)
	}
	return result, nil
}

func (c *ExecServerClient) loadTransport() Transport {
	p := c.transport.Load()
	if p == nil {
		return nil
	}
	return *p
}

func (c *ExecServerClient) storeTransport(t Transport) {
	if t == nil {
		c.transport.Store(nil)
		return
	}
	c.transport.Store(&t)
}

func (c *ExecServerClient) writeMessage(ctx context.Context, payload any) error {
	line, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode JSON-RPC payload: %w", err)
	}

	c.rpcState.lockWrite()
	defer c.rpcState.unlockWrite()

	t := c.loadTransport()
	if t == nil {
		return &TransportClosedError{Message: execServerTransportClosedMessage}
	}
	return t.WriteJSON(ctx, line)
}

func (c *ExecServerClient) readMessage(ctx context.Context, t Transport) (rpcMessage, error) {
	if t == nil {
		return rpcMessage{}, &TransportClosedError{Message: execServerTransportClosedMessage}
	}

	line, err := t.ReadJSON(ctx)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return rpcMessage{}, &TransportClosedError{Message: execServerTransportClosedEOFMessage}
		}
		return rpcMessage{}, err
	}

	var msg rpcMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return rpcMessage{}, &AppServerError{Message: fmt.Sprintf("invalid JSON-RPC line %q: %v", string(line), err)}
	}
	return msg, nil
}

func (c *ExecServerClient) readLoop(ctx context.Context, t Transport, done chan<- struct{}) {
	defer close(done)

	for {
		msg, readErr := c.readMessage(ctx, t)
		if readErr != nil {
			c.failPending(readErr)
			c.closeAllProcessQueues(readErr)

			return
		}

		if msg.Method != "" && msg.ID != "" {
			requestErr := fmt.Errorf("%w: %s", errExecServerUnexpectedServerRequest, msg.Method)
			c.failPending(requestErr)
			c.closeAllProcessQueues(requestErr)

			return
		}

		if msg.Method != "" {
			routeErr := c.routeNotification(Notification{Method: msg.Method, Params: msg.Params})
			if routeErr != nil {
				c.failPending(routeErr)
				c.closeAllProcessQueues(routeErr)

				return
			}

			continue
		}
		c.deliverResponse(msg)
	}
}

func (c *ExecServerClient) deliverResponse(msg rpcMessage) {
	c.rpcState.deliverResponse(msg)
}

func (c *ExecServerClient) failPending(err error) {
	c.rpcState.failPending(err)
}

func (c *ExecServerClient) routeNotification(notification Notification) error {
	switch notification.Method {
	case ExecServerProcessOutputMethod:
		decoded, err := decodeExecServerProcessNotification[ExecServerProcessOutputNotification](notification)
		if err != nil {
			return err
		}
		c.pushProcessNotification(decoded)
	case ExecServerProcessExitedMethod:
		decoded, err := decodeExecServerProcessNotification[ExecServerProcessExitedNotification](notification)
		if err != nil {
			return err
		}
		c.pushProcessNotification(decoded)
	case ExecServerProcessClosedMethod:
		decoded, err := decodeExecServerProcessNotification[ExecServerProcessClosedNotification](notification)
		if err != nil {
			return err
		}
		c.pushProcessNotification(decoded)
	}

	return nil
}

func decodeExecServerProcessNotification[
	ProcessNotification ExecServerProcessNotification,
](notification Notification) (ProcessNotification, error) {
	var got ProcessNotification
	err := json.Unmarshal(notification.Params, &got)
	if err != nil {
		var zero ProcessNotification

		return zero, fmt.Errorf(execServerProcessNotificationDecodeForm, notification.Method, err)
	}
	if got.ProcessIDValue() == "" {
		var zero ProcessNotification

		return zero, fmt.Errorf(
			execServerProcessNotificationDecodeForm,
			notification.Method,
			errExecServerProcessNotificationNoID,
		)
	}
	if got.SeqValue() == 0 {
		var zero ProcessNotification

		return zero, fmt.Errorf(
			execServerProcessNotificationDecodeForm,
			notification.Method,
			errExecServerProcessNotificationNoSeq,
		)
	}

	return got, nil
}

func (c *ExecServerClient) ensureProcessQueue(processID ProcessID) *execServerProcessQueue {
	c.processMu.Lock()
	defer c.processMu.Unlock()

	queue := c.processQueues[processID]
	if queue == nil {
		queue = newExecServerProcessQueue()
		c.processQueues[processID] = queue
	}
	return queue
}

func (c *ExecServerClient) pushProcessNotification(notification ExecServerProcessNotification) {
	c.ensureProcessQueue(notification.ProcessIDValue()).push(notification)
}

func (c *ExecServerClient) nextProcessNotification(ctx context.Context, processID ProcessID, queue *execServerProcessQueue) (ExecServerProcessNotification, error) {
	notification, err := queue.next(ctx)
	if err != nil {
		return nil, err
	}
	if _, ok := notification.(ExecServerProcessClosedNotification); ok {
		queue.close(io.EOF)
		c.deleteProcessQueue(processID, queue)
	}
	return notification, nil
}

func (c *ExecServerClient) deleteProcessQueue(processID ProcessID, queue *execServerProcessQueue) {
	c.processMu.Lock()
	defer c.processMu.Unlock()
	if c.processQueues[processID] == queue {
		delete(c.processQueues, processID)
	}
}

func (c *ExecServerClient) closeAllProcessQueues(err error) {
	c.processMu.Lock()
	defer c.processMu.Unlock()
	for _, queue := range c.processQueues {
		queue.close(err)
	}
	c.processQueues = map[ProcessID]*execServerProcessQueue{}
}

// ExecServerProcessHandle scopes process operations to a single process id.
type ExecServerProcessHandle struct {
	client       *ExecServerClient
	processID    ProcessID
	processQueue *execServerProcessQueue
}

// ID returns the process identifier.
func (h *ExecServerProcessHandle) ID() ProcessID {
	if h == nil {
		return ""
	}
	return h.processID
}

// Read reads retained process output.
func (h *ExecServerProcessHandle) Read(ctx context.Context, params *ExecServerProcessReadParams) (ExecServerProcessReadResponse, error) {
	if h == nil || h.client == nil {
		return ExecServerProcessReadResponse{}, errProcessHandleNil
	}
	scoped := ExecServerProcessReadParams{}
	if params != nil {
		scoped = *params
	}
	scoped.ProcessID = h.processID
	return h.client.ProcessRead(ctx, &scoped)
}

// Write writes to process stdin.
func (h *ExecServerProcessHandle) Write(ctx context.Context, chunk ByteChunk) (ExecServerProcessWriteResponse, error) {
	if h == nil || h.client == nil {
		return ExecServerProcessWriteResponse{}, errProcessHandleNil
	}
	return h.client.ProcessWrite(ctx, &ExecServerProcessWriteParams{ProcessID: h.processID, Chunk: chunk})
}

// Terminate terminates the process.
func (h *ExecServerProcessHandle) Terminate(ctx context.Context) (ExecServerProcessTerminateResponse, error) {
	if h == nil || h.client == nil {
		return ExecServerProcessTerminateResponse{}, errProcessHandleNil
	}
	return h.client.ProcessTerminate(ctx, &ExecServerProcessTerminateParams{ProcessID: h.processID})
}

// NextNotification waits for the next ordered process notification.
func (h *ExecServerProcessHandle) NextNotification(ctx context.Context) (ExecServerProcessNotification, error) {
	if h == nil || h.client == nil {
		return nil, errProcessHandleNil
	}
	if h.processQueue != nil {
		return h.client.nextProcessNotification(ctx, h.processID, h.processQueue)
	}
	return h.client.NextProcessNotification(ctx, h.processID)
}

type execServerProcessQueue struct {
	mu      sync.Mutex
	nextSeq uint64
	pending map[uint64]ExecServerProcessNotification
	notify  chan struct{}
	err     error
}

func newExecServerProcessQueue() *execServerProcessQueue {
	return &execServerProcessQueue{
		nextSeq: 1,
		pending: map[uint64]ExecServerProcessNotification{},
		notify:  make(chan struct{}, 1),
	}
}

func (q *execServerProcessQueue) push(notification ExecServerProcessNotification) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return
	}
	q.pending[notification.SeqValue()] = notification
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *execServerProcessQueue) next(ctx context.Context) (ExecServerProcessNotification, error) {
	for {
		q.mu.Lock()
		if q.err != nil {
			err := q.err
			q.mu.Unlock()
			return nil, err
		}
		if notification, ok := q.pending[q.nextSeq]; ok {
			delete(q.pending, q.nextSeq)
			q.nextSeq++
			q.mu.Unlock()
			return notification, nil
		}
		notify := q.notify
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-notify:
		}
	}
}

func (q *execServerProcessQueue) close(err error) {
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
