// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package broker

import (
	"context"
	"testing"

	corebroker "github.com/absmach/fluxmq/broker"
	"github.com/absmach/fluxmq/mqtt/packets"
	v5 "github.com/absmach/fluxmq/mqtt/packets/v5"
	"github.com/absmach/fluxmq/mqtt/session"
	"github.com/stretchr/testify/require"
)

// qosMutatingHookProvider downgrades the QoS of every publish.
type qosMutatingHookProvider struct {
	qos byte
}

func (p *qosMutatingHookProvider) HandleHook(_ context.Context, req corebroker.BlockingHookRequest) (corebroker.BlockingHookResult, error) {
	if req.Hook == corebroker.HookAuthOnPublish {
		return corebroker.BlockingHookResult{Allowed: true, QoS: p.qos, QoSSet: true}, nil
	}
	return corebroker.BlockingHookResult{Allowed: true}, nil
}

// remappingHookProvider normalizes a fixed alias filter to a mutable target
// and counts/denies unsubscribe hooks on demand.
type remappingHookProvider struct {
	alias         string
	target        string
	denyUnsub     bool
	unsubHookSeen int
}

func (p *remappingHookProvider) HandleHook(_ context.Context, req corebroker.BlockingHookRequest) (corebroker.BlockingHookResult, error) {
	switch req.Hook {
	case corebroker.HookAuthOnSubscribe:
		if req.Topic == p.alias {
			return corebroker.BlockingHookResult{Allowed: true, Topic: p.target}, nil
		}
		return corebroker.BlockingHookResult{Allowed: true}, nil
	case corebroker.HookAuthOnUnsubscribe:
		p.unsubHookSeen++
		return corebroker.BlockingHookResult{Allowed: !p.denyUnsub}, nil
	default:
		return corebroker.BlockingHookResult{Allowed: true}, nil
	}
}

const testAliasFilter = "m/d1/c/ch1/messages"

type denyAllAuthorizer struct{}

func (denyAllAuthorizer) CanPublish(string, string) bool   { return false }
func (denyAllAuthorizer) CanSubscribe(string, string) bool { return false }

func subscribeV5(t *testing.T, h *V5Handler, s *session.Session, filter string, maxQoS byte) {
	t.Helper()
	require.NoError(t, h.HandleSubscribe(s, &v5.Subscribe{
		FixedHeader: packets.FixedHeader{PacketType: packets.SubscribeType, QoS: 1},
		ID:          1,
		Opts:        []v5.SubOption{{Topic: filter, MaxQoS: maxQoS}},
	}))
}

func TestV5PublishHookQoSDowngradeKeepsClientAckFlow(t *testing.T) {
	b := newComplianceTestBroker(t)
	b.SetBlockingHooks(corebroker.NewBlockingHookEngine(&qosMutatingHookProvider{qos: 0}, corebroker.HookFailDeny, nil, nil, nil))
	handler := NewV5Handler(b)

	sub, _, err := b.CreateSession("subscriber", 5, session.Options{CleanStart: true})
	require.NoError(t, err)
	subConn := &captureConnection{}
	require.NoError(t, sub.Connect(subConn))
	subscribeV5(t, handler, sub, "devices/qos", 1)
	subConn.packets = nil

	pub, _, err := b.CreateSession("publisher", 5, session.Options{CleanStart: true})
	require.NoError(t, err)
	pubConn := &captureConnection{}
	require.NoError(t, pub.Connect(pubConn))

	require.NoError(t, handler.HandlePublish(pub, &v5.Publish{
		FixedHeader: packets.FixedHeader{PacketType: packets.PublishType, QoS: 1},
		ID:          42,
		TopicName:   "devices/qos",
		Payload:     []byte("payload"),
	}))

	// The publisher sent QoS 1, so it must receive a PUBACK even though the
	// hook downgraded the delivered QoS to 0.
	require.Len(t, pubConn.packets, 1)
	ack, ok := pubConn.packets[0].(*v5.PubAck)
	require.True(t, ok)
	require.Equal(t, uint16(42), ack.ID)

	require.Len(t, subConn.packets, 1)
	got, ok := subConn.packets[0].(*v5.Publish)
	require.True(t, ok)
	require.Equal(t, byte(0), got.FixedHeader.QoS)
}

func TestV5UnsubscribeAliasRunsHook(t *testing.T) {
	provider := &remappingHookProvider{
		alias:     testAliasFilter,
		target:    "m/domain-id/c/channel-id/messages",
		denyUnsub: true,
	}
	b := newComplianceTestBroker(t)
	b.SetBlockingHooks(corebroker.NewBlockingHookEngine(provider, corebroker.HookFailDeny, nil, nil, nil))
	handler := NewV5Handler(b)

	sub, _, err := b.CreateSession("subscriber", 5, session.Options{CleanStart: true})
	require.NoError(t, err)
	subConn := &captureConnection{}
	require.NoError(t, sub.Connect(subConn))
	subscribeV5(t, handler, sub, provider.alias, 1)
	require.True(t, sub.HasSubscription(provider.target))
	subConn.packets = nil

	require.NoError(t, handler.HandleUnsubscribe(sub, &v5.Unsubscribe{
		FixedHeader: packets.FixedHeader{PacketType: packets.UnsubscribeType, QoS: 1},
		ID:          2,
		Topics:      []string{provider.alias},
	}))

	require.Equal(t, 1, provider.unsubHookSeen)
	require.Len(t, subConn.packets, 1)
	ack, ok := subConn.packets[0].(*v5.UnsubAck)
	require.True(t, ok)
	require.Equal(t, []byte{v5.UnsubAckNotAuthorized}, *ack.ReasonCodes)
	require.True(t, sub.HasSubscription(provider.target))
}

func TestV5SubscribeAliasRemapRemovesStaleSubscription(t *testing.T) {
	provider := &remappingHookProvider{
		alias:  testAliasFilter,
		target: "m/old-domain/c/old-channel/messages",
	}
	b := newComplianceTestBroker(t)
	b.SetBlockingHooks(corebroker.NewBlockingHookEngine(provider, corebroker.HookFailDeny, nil, nil, nil))
	handler := NewV5Handler(b)

	sub, _, err := b.CreateSession("subscriber", 5, session.Options{CleanStart: true})
	require.NoError(t, err)
	subConn := &captureConnection{}
	require.NoError(t, sub.Connect(subConn))

	subscribeV5(t, handler, sub, provider.alias, 1)
	require.True(t, sub.HasSubscription(provider.target))

	provider.target = "m/new-domain/c/new-channel/messages"
	subscribeV5(t, handler, sub, provider.alias, 1)

	require.True(t, sub.HasSubscription(provider.target))
	require.False(t, sub.HasSubscription("m/old-domain/c/old-channel/messages"))
	require.Equal(t, provider.target, sub.ResolveSubscriptionAlias(provider.alias))
}

func TestV5SubscribeAliasNotStoredOnDeniedSubscribe(t *testing.T) {
	provider := &remappingHookProvider{
		alias:  testAliasFilter,
		target: "m/domain-id/c/channel-id/messages",
	}
	b := newComplianceTestBroker(t)
	b.SetAuthEngine(corebroker.NewAuthEngine(nil, denyAllAuthorizer{}))
	b.SetBlockingHooks(corebroker.NewBlockingHookEngine(provider, corebroker.HookFailDeny, nil, nil, nil))
	handler := NewV5Handler(b)

	sub, _, err := b.CreateSession("subscriber", 5, session.Options{CleanStart: true})
	require.NoError(t, err)
	subConn := &captureConnection{}
	require.NoError(t, sub.Connect(subConn))

	subscribeV5(t, handler, sub, provider.alias, 1)

	require.Len(t, subConn.packets, 1)
	ack, ok := subConn.packets[0].(*v5.SubAck)
	require.True(t, ok)
	require.Equal(t, []byte{v5.SubAckNotAuthorized}, *ack.ReasonCodes)
	require.False(t, sub.HasSubscription(provider.target))
	require.Equal(t, provider.alias, sub.ResolveSubscriptionAlias(provider.alias))
}
