// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package mqttsn

import (
	"time"
)

type session struct {
	ClientID     string
	RemoteAddr   string
	CleanSession bool
	Will         bool
	Duration     time.Duration
	LastSeen     time.Time

	topicIDs    map[string]uint16
	topicNames  map[uint16]string
	nextTopicID uint16
}

func newSession(clientID, remoteAddr string, cleanSession, will bool, duration time.Duration, now time.Time) *session {
	return &session{
		ClientID:     clientID,
		RemoteAddr:   remoteAddr,
		CleanSession: cleanSession,
		Will:         will,
		Duration:     duration,
		LastSeen:     now,
		topicIDs:     make(map[string]uint16),
		topicNames:   make(map[uint16]string),
		nextTopicID:  1,
	}
}

func (s *session) touch(now time.Time) {
	s.LastSeen = now
}

func (s *session) expired(now time.Time) bool {
	if s.Duration <= 0 {
		return false
	}
	return now.Sub(s.LastSeen) > s.Duration+s.Duration/2
}

func (s *session) registerTopic(name string) uint16 {
	if id, ok := s.topicIDs[name]; ok {
		return id
	}

	start := s.nextTopicID
	for {
		id := s.nextTopicID
		s.nextTopicID++
		if s.nextTopicID == 0 {
			s.nextTopicID = 1
		}
		if _, exists := s.topicNames[id]; !exists {
			s.topicIDs[name] = id
			s.topicNames[id] = name
			return id
		}
		if s.nextTopicID == start {
			return 0
		}
	}
}

func (s *session) unregisterTopic(name string) {
	id, ok := s.topicIDs[name]
	if !ok {
		return
	}
	delete(s.topicIDs, name)
	delete(s.topicNames, id)
}

func (s *session) topicName(id uint16) (string, bool) {
	name, ok := s.topicNames[id]
	return name, ok
}
