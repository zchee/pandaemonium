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
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/go-json-experiment/json/jsontext"
)

type jsonRPCClientState struct {
	writeMu    sync.Mutex
	responseMu sync.Mutex
	responses  map[string]chan responseWait
	requestSeq atomic.Uint64
}

func newJSONRPCClientState() *jsonRPCClientState {
	return &jsonRPCClientState{responses: map[string]chan responseWait{}}
}

func (s *jsonRPCClientState) nextRequestID() string {
	return fmt.Sprintf("go-sdk-%d", s.requestSeq.Add(1))
}

func (s *jsonRPCClientState) requestRaw(ctx context.Context, method string, params any, write func(context.Context, any) error) (jsontext.Value, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	id := s.nextRequestID()
	response := make(chan responseWait, 1)
	s.registerResponse(id, response)
	if err := write(ctx, Object{"id": id, "method": method, "params": paramsOrEmpty(params)}); err != nil {
		s.unregisterResponse(id)
		return nil, err
	}

	select {
	case <-ctx.Done():
		s.unregisterResponse(id)
		return nil, ctx.Err()
	case got := <-response:
		if got.err != nil {
			return nil, got.err
		}
		if got.msg.Error != nil {
			return nil, mapJSONRPCError(got.msg.Error.Code, got.msg.Error.Message, got.msg.Error.Data)
		}
		return got.msg.Result, nil
	}
}

func (s *jsonRPCClientState) notify(ctx context.Context, method string, params any, write func(context.Context, any) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return write(ctx, Object{"method": method, "params": params})
}

func (s *jsonRPCClientState) registerResponse(id string, response chan responseWait) {
	s.responseMu.Lock()
	s.responses[id] = response
	s.responseMu.Unlock()
}

func (s *jsonRPCClientState) unregisterResponse(id string) {
	s.responseMu.Lock()
	delete(s.responses, id)
	s.responseMu.Unlock()
}

func (s *jsonRPCClientState) deliverResponse(msg *rpcMessage) {
	s.responseMu.Lock()
	response := s.responses[msg.ID]
	delete(s.responses, msg.ID)
	s.responseMu.Unlock()
	if response != nil {
		response <- responseWait{msg: *msg}
	}
}

func (s *jsonRPCClientState) failPending(err error) {
	s.responseMu.Lock()
	responses := s.responses
	s.responses = map[string]chan responseWait{}
	s.responseMu.Unlock()
	for _, response := range responses {
		response <- responseWait{err: err}
	}
}

func (s *jsonRPCClientState) lockWrite() {
	s.writeMu.Lock()
}

func (s *jsonRPCClientState) unlockWrite() {
	s.writeMu.Unlock()
}
