// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"context"
	"testing"

	"github.com/absmach/fluxmq/queue/types"
	"github.com/stretchr/testify/require"
)

func TestGroupStoreConsumerMembership(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t, "development")
	groups := NewGroupStore(store)
	group := types.NewConsumerGroupState("development", "workers", "")
	require.NoError(t, groups.CreateConsumerGroup(ctx, group))

	consumer := &types.ConsumerInfo{ID: "worker-1"}
	require.NoError(t, groups.RegisterConsumer(ctx, "development", "workers", consumer))
	listed, err := groups.ListConsumers(ctx, "development", "workers")
	require.NoError(t, err)
	require.Equal(t, []*types.ConsumerInfo{consumer}, listed)

	require.NoError(t, groups.UnregisterConsumer(ctx, "development", "workers", consumer.ID))
	listed, err = groups.ListConsumers(ctx, "development", "workers")
	require.NoError(t, err)
	require.Empty(t, listed)
}
