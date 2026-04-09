// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package authcallout

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/absmach/fluxmq/broker"
	authv1 "github.com/absmach/fluxmq/pkg/proto/auth/v1"
	"github.com/absmach/fluxmq/pkg/proto/auth/v1/authv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAuthServer struct {
	authnResult *authv1.AuthnRes
	authzResult *authv1.AuthzRes
	authnErr    error
	authzErr    error
}

func (f *fakeAuthServer) Authenticate(_ context.Context, req *connect.Request[authv1.AuthnReq]) (*connect.Response[authv1.AuthnRes], error) {
	if f.authnErr != nil {
		return nil, f.authnErr
	}
	return connect.NewResponse(f.authnResult), nil
}

func (f *fakeAuthServer) Authorize(_ context.Context, req *connect.Request[authv1.AuthzReq]) (*connect.Response[authv1.AuthzRes], error) {
	if f.authzErr != nil {
		return nil, f.authzErr
	}
	return connect.NewResponse(f.authzResult), nil
}

func startTestServer(t *testing.T, handler authv1connect.AuthServiceHandler) (*httptest.Server, *GRPCClient) {
	t.Helper()
	mux := http.NewServeMux()
	path, h := authv1connect.NewAuthServiceHandler(handler)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := NewGRPCClient(srv.Client(), srv.URL,
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
	return srv, client
}

func TestGRPCClient_Authenticate_Success(t *testing.T) {
	_, client := startTestServer(t, &fakeAuthServer{
		authnResult: &authv1.AuthnRes{
			Authenticated: true,
			Id:            "ext-id-1",
		},
	})

	result, err := client.Authenticate("mqtt-client", "user", "pass")
	require.NoError(t, err)
	assert.True(t, result.Authenticated)
	assert.Equal(t, "ext-id-1", result.ID)
}

func TestGRPCClient_Authenticate_Denied(t *testing.T) {
	_, client := startTestServer(t, &fakeAuthServer{
		authnResult: &authv1.AuthnRes{
			Authenticated: false,
			ReasonCode:    2,
			Reason:        "bad credentials",
		},
	})

	result, err := client.Authenticate("mqtt-client", "user", "wrong")
	require.NoError(t, err)
	assert.False(t, result.Authenticated)
}

func TestGRPCClient_Authenticate_ServerError(t *testing.T) {
	_, client := startTestServer(t, &fakeAuthServer{
		authnErr: connect.NewError(connect.CodeInternal, nil),
	})

	result, err := client.Authenticate("mqtt-client", "user", "pass")
	require.Error(t, err)
	assert.False(t, result.Authenticated)
}

func TestGRPCClient_CanPublish_Allowed(t *testing.T) {
	_, client := startTestServer(t, &fakeAuthServer{
		authzResult: &authv1.AuthzRes{Authorized: true},
	})

	assert.True(t, client.CanPublish("ext-id-1", "m/domain/c/channel/temp"))
}

func TestGRPCClient_CanPublish_Denied(t *testing.T) {
	_, client := startTestServer(t, &fakeAuthServer{
		authzResult: &authv1.AuthzRes{
			Authorized: false,
			ReasonCode: 4,
			Reason:     "not authorized",
		},
	})

	assert.False(t, client.CanPublish("ext-id-1", "m/domain/c/channel/temp"))
}

func TestGRPCClient_CanSubscribe_Allowed(t *testing.T) {
	_, client := startTestServer(t, &fakeAuthServer{
		authzResult: &authv1.AuthzRes{Authorized: true},
	})

	assert.True(t, client.CanSubscribe("ext-id-1", "m/domain/c/channel/#"))
}

func TestGRPCClient_CanSubscribe_ServerError(t *testing.T) {
	_, client := startTestServer(t, &fakeAuthServer{
		authzErr: connect.NewError(connect.CodeUnavailable, nil),
	})

	assert.False(t, client.CanSubscribe("ext-id-1", "m/domain/c/channel"))
}

func TestGRPCClient_ImplementsInterfaces(t *testing.T) {
	_, client := startTestServer(t, &fakeAuthServer{
		authnResult: &authv1.AuthnRes{Authenticated: true, Id: "x"},
		authzResult: &authv1.AuthzRes{Authorized: true},
	})

	var _ broker.Authenticator = client
	var _ broker.Authorizer = client
}

// transientServer fails the first failFor calls with the given code, then
// delegates to the underlying handler.
type transientServer struct {
	authv1connect.UnimplementedAuthServiceHandler
	authnFailFor atomic.Int32
	authzFailFor atomic.Int32
	code         connect.Code
	delegate     authv1connect.AuthServiceHandler
}

func (s *transientServer) Authenticate(ctx context.Context, req *connect.Request[authv1.AuthnReq]) (*connect.Response[authv1.AuthnRes], error) {
	if s.authnFailFor.Add(-1) >= 0 {
		return nil, connect.NewError(s.code, nil)
	}
	return s.delegate.Authenticate(ctx, req)
}

func (s *transientServer) Authorize(ctx context.Context, req *connect.Request[authv1.AuthzReq]) (*connect.Response[authv1.AuthzRes], error) {
	if s.authzFailFor.Add(-1) >= 0 {
		return nil, connect.NewError(s.code, nil)
	}
	return s.delegate.Authorize(ctx, req)
}

func startTransientServer(t *testing.T, failFor int, code connect.Code, delegate authv1connect.AuthServiceHandler) *GRPCClient {
	t.Helper()
	srv := &transientServer{code: code, delegate: delegate}
	srv.authnFailFor.Store(int32(failFor))
	srv.authzFailFor.Store(int32(failFor))
	_, client := startTestServer(t, srv)
	return client
}

func TestGRPCClient_Authenticate_RetryOnTransient(t *testing.T) {
	// Server fails once with Unavailable, then succeeds. With 2 attempts the
	// client should succeed and the circuit breaker should not be tripped.
	delegate := &fakeAuthServer{authnResult: &authv1.AuthnRes{Authenticated: true, Id: "retried-id"}}
	client := startTransientServer(t, 1, connect.CodeUnavailable, delegate)
	client.RetryBackoff = 0 // no sleep in tests

	result, err := client.Authenticate("mqtt-client", "user", "pass")
	require.NoError(t, err)
	assert.True(t, result.Authenticated)
	assert.Equal(t, "retried-id", result.ID)
	assert.Equal(t, 0, int(client.CB.Counts().TotalFailures), "CB should see no failures after successful retry")
}

func TestGRPCClient_CanPublish_RetryOnTransient(t *testing.T) {
	delegate := &fakeAuthServer{authzResult: &authv1.AuthzRes{Authorized: true}}
	client := startTransientServer(t, 1, connect.CodeUnavailable, delegate)
	client.RetryBackoff = 0

	assert.True(t, client.CanPublish("ext-id", "m/domain/c/channel/temp"))
	assert.Equal(t, 0, int(client.CB.Counts().TotalFailures))
}

func TestGRPCClient_Authenticate_NoRetryOnPermanent(t *testing.T) {
	// Unauthenticated is a permanent error — should not retry.
	srv := &transientServer{
		code: connect.CodeUnauthenticated,
		delegate: &fakeAuthServer{
			authnResult: &authv1.AuthnRes{Authenticated: true},
		},
	}
	srv.authnFailFor.Store(99)
	mux := http.NewServeMux()
	path, h := authv1connect.NewAuthServiceHandler(srv)
	mux.Handle(path, h)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := NewGRPCClient(ts.Client(), ts.URL,
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		WithRetryAttempts(3),
		WithRetryBackoff(0*time.Millisecond),
	)
	_, err := client.Authenticate("mqtt-client", "user", "pass")
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestGRPCClient_Authenticate_ExhaustsRetries(t *testing.T) {
	// Server always returns Unavailable. Client should exhaust retries and return error.
	client := startTransientServer(t, 99, connect.CodeUnavailable,
		&fakeAuthServer{authnResult: &authv1.AuthnRes{Authenticated: true}},
	)
	client.RetryAttempts = 3
	client.RetryBackoff = 0

	_, err := client.Authenticate("mqtt-client", "user", "pass")
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
}

func TestWithRetryAttempts(t *testing.T) {
	calls := 0
	_, err := withRetry(3, 0, func() (any, error) {
		calls++
		return nil, connect.NewError(connect.CodeUnavailable, nil)
	})
	require.Error(t, err)
	assert.Equal(t, 3, calls, "should have made exactly 3 attempts")
}

func TestWithRetrySucceedsOnSecondAttempt(t *testing.T) {
	calls := 0
	result, err := withRetry(3, 0, func() (any, error) {
		calls++
		if calls < 2 {
			return nil, connect.NewError(connect.CodeUnavailable, nil)
		}
		return 42, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 42, result)
	assert.Equal(t, 2, calls)
}

func TestWithRetryNoRetryOnPermanent(t *testing.T) {
	calls := 0
	_, err := withRetry(3, 0, func() (any, error) {
		calls++
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	})
	require.Error(t, err)
	assert.Equal(t, 1, calls, "permanent error should not be retried")
}

func TestWithRetryBackoff(t *testing.T) {
	start := time.Now()
	_, _ = withRetry(3, 20*time.Millisecond, func() (any, error) {
		return nil, connect.NewError(connect.CodeUnavailable, nil)
	})
	elapsed := time.Since(start)
	// 3 attempts = 2 sleeps of 20ms each
	assert.GreaterOrEqual(t, elapsed, 40*time.Millisecond)
}
