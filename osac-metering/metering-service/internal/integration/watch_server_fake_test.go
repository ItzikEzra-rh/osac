/*
Copyright (c) 2026 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except
in compliance with the License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0
*/

package integration_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/osac-project/osac-metering/internal/projection"
	"github.com/osac-project/osac-metering/internal/watch"
	privatev1 "github.com/osac-project/osac/proto/gen/osac/private/v1"
)

type watchServerFake struct {
	privatev1.UnimplementedEventsServer
	event     *privatev1.Event
	requests  chan *privatev1.EventsWatchRequest
	callCount atomic.Int32
}

var _ privatev1.EventsServer = (*watchServerFake)(nil)

func (s *watchServerFake) Watch(
	request *privatev1.EventsWatchRequest,
	stream grpc.ServerStreamingServer[privatev1.EventsWatchResponse],
) error {
	s.callCount.Add(1)
	select {
	case s.requests <- request:
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
	if err := stream.Send(&privatev1.EventsWatchResponse{Event: s.event}); err != nil {
		return err
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}

type memoryProjectionStore struct {
	mu     sync.RWMutex
	states map[string]projection.ResourceState
}

var _ projection.Store = (*memoryProjectionStore)(nil)

func newMemoryProjectionStore() *memoryProjectionStore {
	return &memoryProjectionStore{states: make(map[string]projection.ResourceState)}
}

func (s *memoryProjectionStore) Get(_ context.Context, resourceID string) (*projection.ResourceState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.states[resourceID]
	if !ok {
		return nil, nil
	}
	return &state, nil
}

func (s *memoryProjectionStore) Upsert(_ context.Context, state projection.ResourceState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.states[state.ResourceID]; ok && existing.FulfillmentVersion > state.FulfillmentVersion {
		return projection.ErrStaleVersion
	}
	s.states[state.ResourceID] = state
	return nil
}

func (s *memoryProjectionStore) Delete(_ context.Context, resourceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, resourceID)
	return nil
}

func (s *memoryProjectionStore) ListBillable(_ context.Context) ([]projection.ResourceState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []projection.ResourceState
	for _, state := range s.states {
		if state.IsBillable {
			result = append(result, state)
		}
	}
	return result, nil
}

func (s *memoryProjectionStore) ListAll(_ context.Context) ([]projection.ResourceState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]projection.ResourceState, 0, len(s.states))
	for _, state := range s.states {
		result = append(result, state)
	}
	return result, nil
}

func (*memoryProjectionStore) UpdateLastHeartbeat(_ context.Context, _ []string, _ time.Time) error {
	return nil
}

type unusedExternalIPPoolGetter struct{}

var _ watch.ExternalIPPoolGetter = unusedExternalIPPoolGetter{}

func (unusedExternalIPPoolGetter) Get(
	context.Context,
	*privatev1.ExternalIPPoolsGetRequest,
	...grpc.CallOption,
) (*privatev1.ExternalIPPoolsGetResponse, error) {
	return nil, errors.New("unexpected external IP pool lookup")
}
