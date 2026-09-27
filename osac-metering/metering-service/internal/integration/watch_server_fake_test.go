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
	"sync/atomic"

	"google.golang.org/grpc"

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

type unusedExternalIPPoolGetter struct{}

var _ watch.ExternalIPPoolGetter = unusedExternalIPPoolGetter{}

func (unusedExternalIPPoolGetter) Get(
	context.Context,
	*privatev1.ExternalIPPoolsGetRequest,
	...grpc.CallOption,
) (*privatev1.ExternalIPPoolsGetResponse, error) {
	return nil, errors.New("unexpected external IP pool lookup")
}
