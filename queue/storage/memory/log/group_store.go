// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"context"

	"github.com/absmach/fluxmq/queue/storage"
	"github.com/absmach/fluxmq/queue/types"
)

var _ storage.ConsumerGroupStore = (*GroupStore)(nil)

// GroupStore exposes the queue store's in-memory consumer-group state through
// the current ConsumerGroupStore interface. It is a separate adapter because
// Store also implements the legacy ConsumerStore interface, whose ListConsumers
// method intentionally returns a different type.
type GroupStore struct {
	store *Store
}

// NewGroupStore creates an in-memory consumer-group store sharing state with
// the supplied queue store.
func NewGroupStore(store *Store) *GroupStore {
	return &GroupStore{store: store}
}

func (s *GroupStore) CreateConsumerGroup(ctx context.Context, group *types.ConsumerGroup) error {
	return s.store.CreateConsumerGroup(ctx, group)
}

func (s *GroupStore) GetConsumerGroup(ctx context.Context, queueName, groupID string) (*types.ConsumerGroup, error) {
	return s.store.GetConsumerGroup(ctx, queueName, groupID)
}

func (s *GroupStore) UpdateConsumerGroup(ctx context.Context, group *types.ConsumerGroup) error {
	return s.store.UpdateConsumerGroup(ctx, group)
}

func (s *GroupStore) DeleteConsumerGroup(ctx context.Context, queueName, groupID string) error {
	return s.store.DeleteConsumerGroup(ctx, queueName, groupID)
}

func (s *GroupStore) ListConsumerGroups(ctx context.Context, queueName string) ([]*types.ConsumerGroup, error) {
	return s.store.ListConsumerGroups(ctx, queueName)
}

func (s *GroupStore) AddPendingEntry(ctx context.Context, queueName, groupID string, entry *types.PendingEntry) error {
	return s.store.AddPendingEntry(ctx, queueName, groupID, entry)
}

func (s *GroupStore) RemovePendingEntry(ctx context.Context, queueName, groupID, consumerID string, offset uint64) error {
	return s.store.RemovePendingEntry(ctx, queueName, groupID, consumerID, offset)
}

func (s *GroupStore) GetPendingEntries(ctx context.Context, queueName, groupID, consumerID string) ([]*types.PendingEntry, error) {
	return s.store.GetPendingEntries(ctx, queueName, groupID, consumerID)
}

func (s *GroupStore) GetAllPendingEntries(ctx context.Context, queueName, groupID string) ([]*types.PendingEntry, error) {
	return s.store.GetAllPendingEntries(ctx, queueName, groupID)
}

func (s *GroupStore) TransferPendingEntry(ctx context.Context, queueName, groupID string, offset uint64, fromConsumer, toConsumer string) error {
	return s.store.TransferPendingEntry(ctx, queueName, groupID, offset, fromConsumer, toConsumer)
}

func (s *GroupStore) UpdateCursor(ctx context.Context, queueName, groupID string, cursor uint64) error {
	return s.store.UpdateCursor(ctx, queueName, groupID, cursor)
}

func (s *GroupStore) UpdateCommitted(ctx context.Context, queueName, groupID string, committed uint64) error {
	return s.store.UpdateCommitted(ctx, queueName, groupID, committed)
}

func (s *GroupStore) RegisterConsumer(ctx context.Context, queueName, groupID string, consumer *types.ConsumerInfo) error {
	group, err := s.store.GetConsumerGroup(ctx, queueName, groupID)
	if err != nil {
		return err
	}
	group.SetConsumer(consumer.ID, consumer)
	return nil
}

func (s *GroupStore) UnregisterConsumer(ctx context.Context, queueName, groupID, consumerID string) error {
	group, err := s.store.GetConsumerGroup(ctx, queueName, groupID)
	if err != nil {
		return err
	}
	group.DeleteConsumer(consumerID)
	return nil
}

func (s *GroupStore) ListConsumers(ctx context.Context, queueName, groupID string) ([]*types.ConsumerInfo, error) {
	group, err := s.store.GetConsumerGroup(ctx, queueName, groupID)
	if err != nil {
		return nil, err
	}
	consumers := make([]*types.ConsumerInfo, 0, group.ConsumerCount())
	group.ForEachConsumer(func(_ string, info *types.ConsumerInfo) bool {
		consumers = append(consumers, info)
		return true
	})
	return consumers, nil
}
