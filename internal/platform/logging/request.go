package logging

import (
	"context"
	"sync"
)

type requestLogStateKey struct{}

// RequestFailure carries safe request diagnostics to the shared access logger.
type RequestFailure struct {
	Status        int
	Code          string
	Err           error
	AfterResponse bool
}

// RequestLogState collects one failure record for a single HTTP request.
type RequestLogState struct {
	mu      sync.Mutex
	failure *RequestFailure
}

// WithRequestLogState adds a request-local failure collector to ctx.
func WithRequestLogState(ctx context.Context) (context.Context, *RequestLogState) {
	state := &RequestLogState{}
	return context.WithValue(ctx, requestLogStateKey{}, state), state
}

// RequestLogStateFrom returns the request-local failure collector, if present.
func RequestLogStateFrom(ctx context.Context) *RequestLogState {
	state, _ := ctx.Value(requestLogStateKey{}).(*RequestLogState)
	return state
}

// RecordFailure keeps the first failure so one request produces one diagnostic.
func (s *RequestLogState) RecordFailure(failure RequestFailure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure == nil {
		s.failure = &failure
	}
}

// Failure returns the request failure recorded so far.
func (s *RequestLogState) Failure() (RequestFailure, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure == nil {
		return RequestFailure{}, false
	}
	return *s.failure, true
}
