/*
Copyright (c) 2026 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except
in compliance with the License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0
*/

// Package testutil contains shared test doubles for the metering service.
package testutil

import (
	"context"
	"sync"
	"time"

	"github.com/osac-project/osac-metering/internal/projection"
)

// MemoryProjectionStore implements the projection store contract for tests.
// The provided map can be retained by a package-local wrapper for direct fixture assertions.
type MemoryProjectionStore struct {
	mu                  sync.RWMutex
	states              map[string]projection.ResourceState
	rejectStaleVersions bool
}

var _ projection.Store = (*MemoryProjectionStore)(nil)

// NewMemoryProjectionStore uses states as its backing map and accepts every upsert.
func NewMemoryProjectionStore(states map[string]projection.ResourceState) *MemoryProjectionStore {
	return newMemoryProjectionStore(states, false)
}

// NewMonotonicMemoryProjectionStore rejects upserts older than the stored version.
func NewMonotonicMemoryProjectionStore(states map[string]projection.ResourceState) *MemoryProjectionStore {
	return newMemoryProjectionStore(states, true)
}

// Lock lets a test synchronize direct access to the backing map it supplied to the constructor.
func (s *MemoryProjectionStore) Lock() {
	s.mu.Lock()
}

// Unlock releases the lock acquired by Lock.
func (s *MemoryProjectionStore) Unlock() {
	s.mu.Unlock()
}

func newMemoryProjectionStore(
	states map[string]projection.ResourceState,
	rejectStaleVersions bool,
) *MemoryProjectionStore {
	if states == nil {
		states = make(map[string]projection.ResourceState)
	}
	return &MemoryProjectionStore{
		states:              states,
		rejectStaleVersions: rejectStaleVersions,
	}
}

func (s *MemoryProjectionStore) Get(_ context.Context, resourceID string) (*projection.ResourceState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.states[resourceID]
	if !ok {
		return nil, nil
	}
	return &state, nil
}

func (s *MemoryProjectionStore) Upsert(_ context.Context, state projection.ResourceState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.states[state.ResourceID]; s.rejectStaleVersions && ok && existing.FulfillmentVersion > state.FulfillmentVersion {
		return projection.ErrStaleVersion
	}
	s.states[state.ResourceID] = state
	return nil
}

func (s *MemoryProjectionStore) Delete(_ context.Context, resourceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, resourceID)
	return nil
}

func (s *MemoryProjectionStore) ListBillable(_ context.Context) ([]projection.ResourceState, error) {
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

func (s *MemoryProjectionStore) ListAll(_ context.Context) ([]projection.ResourceState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]projection.ResourceState, 0, len(s.states))
	for _, state := range s.states {
		result = append(result, state)
	}
	return result, nil
}

func (*MemoryProjectionStore) UpdateLastHeartbeat(_ context.Context, _ []string, _ time.Time) error {
	return nil
}
