// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package authcallout

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/absmach/fluxmq/broker"
	authv1 "github.com/absmach/fluxmq/pkg/proto/auth/v1"
	"github.com/absmach/fluxmq/pkg/proto/auth/v1/authv1connect"
	"golang.org/x/net/http2"
)

var (
	_ broker.Authenticator = (*GRPCClient)(nil)
	_ broker.Authorizer    = (*GRPCClient)(nil)
)

// GRPCClient implements broker.Authenticator and broker.Authorizer by calling
// a remote AuthService over ConnectRPC (gRPC-compatible).
type GRPCClient struct {
	svc authv1connect.AuthServiceClient
	Options
}

// NewGRPCClient creates a callout client that dials the given base URL.
// The URL should be the ConnectRPC/gRPC server address
// (e.g. "http://localhost:9090").
// When httpClient is nil, an appropriate client is built from the URL scheme:
// http:// gets an h2c (cleartext HTTP/2) transport with read-idle pings so
// that server-closed connections are detected promptly without relying on
// long timeouts; https:// gets the standard transport with TLS.
func NewGRPCClient(httpClient *http.Client, baseURL string, opts ...Option) *GRPCClient {
	if httpClient == nil {
		httpClient = client(baseURL)
	}

	o := DefaultOptions(opts...)
	return &GRPCClient{
		svc:     authv1connect.NewAuthServiceClient(httpClient, baseURL, connect.WithGRPC()),
		Options: o,
	}
}

// client returns an HTTP client suitable for the given URL scheme.
// For h2c (http://) it configures an HTTP/2 transport with cleartext dialing
// and read-idle pings; for everything else it returns http.DefaultClient.
func client(baseURL string) *http.Client {
	u, err := url.Parse(baseURL)

	if err != nil || u.Scheme != "http" {
		return http.DefaultClient
	}
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
			// Send a ping after 10 s of inactivity so a server-side idle
			// close is detected before the next auth call fails.
			ReadIdleTimeout: 10 * time.Second,
			PingTimeout:     5 * time.Second,
		},
	}
}

// Authenticate calls the remote AuthService.Authenticate RPC.
func (c *GRPCClient) Authenticate(clientID, username, secret string) (*broker.AuthnResult, error) {
	req := connect.NewRequest(&authv1.AuthnReq{
		ClientId: clientID,
		Username: username,
		Password: secret,
		Protocol: c.Protocol,
	})

	result, err := c.CB.Execute(func() (any, error) {
		return withRetry(c.RetryAttempts, c.RetryBackoff, func() (any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
			defer cancel()
			return c.svc.Authenticate(ctx, req)
		})
	})
	if err != nil {
		c.Logger.Info("auth_callout_authenticate",
			slog.String("client_id", clientID),
			slog.String("status", "error"),
			slog.String("error", err.Error()))
		return &broker.AuthnResult{}, err
	}

	res := result.(*connect.Response[authv1.AuthnRes])
	msg := res.Msg
	c.Logger.Info("auth_callout_authenticate",
		slog.String("client_id", clientID),
		slog.String("status", "ok"),
		slog.Bool("authenticated", msg.GetAuthenticated()))

	return &broker.AuthnResult{
		Authenticated: msg.GetAuthenticated(),
		ID:            msg.GetId(),
	}, nil
}

// CanPublish calls the remote AuthService.Authorize RPC for publish.
func (c *GRPCClient) CanPublish(clientID string, topic string) bool {
	return c.authorize(clientID, topic, authv1.Action_Publish)
}

// CanSubscribe calls the remote AuthService.Authorize RPC for subscribe.
func (c *GRPCClient) CanSubscribe(clientID string, filter string) bool {
	return c.authorize(clientID, filter, authv1.Action_Subscribe)
}

func (c *GRPCClient) authorize(externalID, topic string, action authv1.Action) bool {
	req := connect.NewRequest(&authv1.AuthzReq{
		ExternalId: externalID,
		Topic:      topic,
		Action:     action,
	})

	result, err := c.CB.Execute(func() (any, error) {
		return withRetry(c.RetryAttempts, c.RetryBackoff, func() (any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
			defer cancel()
			return c.svc.Authorize(ctx, req)
		})
	})
	if err != nil {
		c.Logger.Info("auth_callout_authorize",
			slog.String("external_id", externalID),
			slog.String("topic", topic),
			slog.String("action", action.String()),
			slog.String("status", "error"),
			slog.String("error", err.Error()))
		return false
	}

	res := result.(*connect.Response[authv1.AuthzRes])
	msg := res.Msg
	c.Logger.Info("auth_callout_authorize",
		slog.String("external_id", externalID),
		slog.String("topic", topic),
		slog.String("action", action.String()),
		slog.String("status", "ok"),
		slog.Bool("authorized", msg.GetAuthorized()))

	return msg.GetAuthorized()
}

// withRetry calls fn up to attempts times, retrying only on transient errors.
// Transient error are used to prevent CB to fail fast, causing opening new
// connection or retries before making sure the error is not unstable connection.
// Each attempt gets its own per-call timeout via context created inside fn.
// Retries are transparent to the circuit breaker: only a non-transient error
// or exhausted retries propagates out as a CB failure.
func withRetry(attempts int, backoff time.Duration, fn func() (any, error)) (any, error) {
	var err error
	for i := 0; i < attempts; i++ {
		var result any
		result, err = fn()
		if err == nil {
			return result, nil
		}
		if !isTransientConnectError(err) || i == attempts-1 {
			break
		}
		time.Sleep(backoff)
	}
	return nil, err
}

// isTransientConnectError reports whether err represents a transient failure
// that is safe to retry (connection drop, server temporarily unavailable, etc.).
// Permanent errors such as Unauthenticated or PermissionDenied return false.
func isTransientConnectError(err error) bool {
	if err == nil {
		return false
	}

	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		switch connectErr.Code() {
		case connect.CodeUnavailable, connect.CodeDeadlineExceeded,
			connect.CodeAborted, connect.CodeResourceExhausted:
			return true
		default:
			return false
		}
	}

	// Network-level errors that surface before the connect layer.
	msg := strings.ToLower(err.Error())
	for _, frag := range []string{
		"eof", "connection reset", "broken pipe", "connection refused",
		"transport is closing", "timed out", "no such host",
		"http2: client connection lost",
	} {
		if strings.Contains(msg, frag) {
			return true
		}
	}

	return false
}
