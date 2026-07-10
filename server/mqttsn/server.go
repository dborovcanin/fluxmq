// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package mqttsn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/absmach/fluxmq/internal/connguard"
	"github.com/absmach/fluxmq/mqtt/packets/sn"
	"github.com/absmach/fluxmq/topics"
)

const defaultMaxPacketSize = 65535

// Config holds the MQTT-SN UDP listener configuration.
type Config struct {
	Address         string
	ShutdownTimeout time.Duration
	MaxPacketSize   int
}

// Server handles MQTT-SN 1.2 datagrams over UDP.
//
// This is the protocol-layer skeleton. It handles sessions, keepalive, and
// topic-id registration, but does not yet bridge publishes/subscriptions into
// the broker router.
type Server struct {
	config Config
	logger *slog.Logger
	now    func() time.Time

	mu       sync.Mutex
	sessions map[string]*session
}

// New creates an MQTT-SN UDP server.
func New(cfg Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 30 * time.Second
	}
	if cfg.MaxPacketSize <= 0 {
		cfg.MaxPacketSize = defaultMaxPacketSize
	}

	return &Server{
		config:   cfg,
		logger:   logger,
		now:      time.Now,
		sessions: make(map[string]*session),
	}
}

// Listen starts the MQTT-SN UDP listener and blocks until ctx is cancelled.
func (s *Server) Listen(ctx context.Context) error {
	conn, err := net.ListenPacket("udp", s.config.Address)
	if err != nil {
		return fmt.Errorf("failed to create MQTT-SN UDP listener: %w", err)
	}
	defer conn.Close()

	s.logger.Info("mqttsn_udp_server_started", slog.String("addr", s.config.Address))

	closeDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closeDone:
		}
	}()
	defer close(closeDone)

	buf := make([]byte, s.config.MaxPacketSize)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				s.logger.Info("mqttsn_udp_server_stopped", slog.String("addr", s.config.Address))
				return nil
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Temporary() {
				s.logger.Warn("mqttsn_udp_read_temporary_error", slog.String("error", err.Error()))
				continue
			}
			return fmt.Errorf("MQTT-SN UDP server error: %w", err)
		}

		data := make([]byte, n)
		copy(data, buf[:n])
		s.handleDatagram(conn, addr, data)
	}
}

func (s *Server) handleDatagram(conn net.PacketConn, addr net.Addr, data []byte) {
	defer connguard.Recover(s.logger, "mqtt-sn-udp", addr.String())

	if len(data) > s.config.MaxPacketSize {
		s.logger.Warn("mqttsn_packet_too_large",
			slog.String("remote", addr.String()),
			slog.Int("size", len(data)),
			slog.Int("max_packet_size", s.config.MaxPacketSize))
		return
	}

	pkt, err := sn.DecodePacket(data)
	if err != nil {
		s.logger.Warn("mqttsn_decode_failed",
			slog.String("remote", addr.String()),
			slog.String("error", err.Error()))
		return
	}

	s.pruneExpired()

	switch p := pkt.(type) {
	case *sn.Connect:
		s.handleConnect(conn, addr, p)
	case *sn.PingReq:
		s.handlePingReq(conn, addr, p)
	case *sn.Disconnect:
		s.handleDisconnect(addr, p)
	case *sn.Register:
		s.handleRegister(conn, addr, p)
	case *sn.Subscribe:
		s.handleSubscribe(conn, addr, p)
	case *sn.Unsubscribe:
		s.handleUnsubscribe(conn, addr, p)
	case *sn.Publish:
		s.handlePublish(conn, addr, p)
	default:
		s.logger.Debug("mqttsn_packet_ignored",
			slog.String("remote", addr.String()),
			slog.String("type", sn.PacketNames[pkt.Type()]))
	}
}

func (s *Server) handleConnect(conn net.PacketConn, addr net.Addr, pkt *sn.Connect) {
	if pkt.ProtocolID != sn.ProtocolID || pkt.ClientID == "" {
		s.writePacket(conn, addr, &sn.ConnAck{ReturnCode: sn.ReturnRejectedNotSupported})
		return
	}

	duration := time.Duration(pkt.Duration) * time.Second
	key := addr.String()
	now := s.now()

	s.mu.Lock()
	s.sessions[key] = newSession(pkt.ClientID, key, pkt.Flags.CleanSession, pkt.Flags.Will, duration, now)
	s.mu.Unlock()

	s.writePacket(conn, addr, &sn.ConnAck{ReturnCode: sn.ReturnAccepted})
}

func (s *Server) handlePingReq(conn net.PacketConn, addr net.Addr, _ *sn.PingReq) {
	if sess := s.getSession(addr); sess != nil {
		sess.touch(s.now())
	}
	s.writePacket(conn, addr, &sn.PingResp{})
}

func (s *Server) handleDisconnect(addr net.Addr, _ *sn.Disconnect) {
	s.mu.Lock()
	delete(s.sessions, addr.String())
	s.mu.Unlock()
}

func (s *Server) handleRegister(conn net.PacketConn, addr net.Addr, pkt *sn.Register) {
	sess := s.getSession(addr)
	if sess == nil {
		s.writePacket(conn, addr, &sn.RegAck{
			TopicID:    pkt.TopicID,
			MessageID:  pkt.MessageID,
			ReturnCode: sn.ReturnRejectedNotSupported,
		})
		return
	}
	if err := topics.ValidateTopicName(pkt.TopicName); err != nil {
		s.writePacket(conn, addr, &sn.RegAck{
			TopicID:    pkt.TopicID,
			MessageID:  pkt.MessageID,
			ReturnCode: sn.ReturnRejectedInvalidTopicID,
		})
		return
	}

	s.mu.Lock()
	topicID := sess.registerTopic(pkt.TopicName)
	sess.touch(s.now())
	s.mu.Unlock()

	if topicID == 0 {
		s.writePacket(conn, addr, &sn.RegAck{
			TopicID:    pkt.TopicID,
			MessageID:  pkt.MessageID,
			ReturnCode: sn.ReturnRejectedCongestion,
		})
		return
	}

	s.writePacket(conn, addr, &sn.RegAck{
		TopicID:    topicID,
		MessageID:  pkt.MessageID,
		ReturnCode: sn.ReturnAccepted,
	})
}

func (s *Server) handleSubscribe(conn net.PacketConn, addr net.Addr, pkt *sn.Subscribe) {
	sess := s.getSession(addr)
	if sess == nil || pkt.Flags.QoS == sn.QoSMinusOne {
		s.writePacket(conn, addr, &sn.SubAck{
			Flags:      sn.Flags{QoS: pkt.Flags.QoS, TopicIDType: pkt.Topic.Type},
			MessageID:  pkt.MessageID,
			ReturnCode: sn.ReturnRejectedNotSupported,
		})
		return
	}

	if pkt.Topic.Type != sn.TopicIDTypeNormal {
		s.writePacket(conn, addr, &sn.SubAck{
			Flags:      sn.Flags{QoS: pkt.Flags.QoS, TopicIDType: pkt.Topic.Type},
			MessageID:  pkt.MessageID,
			ReturnCode: sn.ReturnRejectedNotSupported,
		})
		return
	}
	if err := topics.ValidateTopicFilter(pkt.Topic.Name); err != nil {
		s.writePacket(conn, addr, &sn.SubAck{
			Flags:      sn.Flags{QoS: pkt.Flags.QoS, TopicIDType: pkt.Topic.Type},
			MessageID:  pkt.MessageID,
			ReturnCode: sn.ReturnRejectedInvalidTopicID,
		})
		return
	}

	s.mu.Lock()
	topicID := sess.registerTopic(pkt.Topic.Name)
	sess.touch(s.now())
	s.mu.Unlock()

	if topicID == 0 {
		s.writePacket(conn, addr, &sn.SubAck{
			Flags:      sn.Flags{QoS: pkt.Flags.QoS, TopicIDType: pkt.Topic.Type},
			MessageID:  pkt.MessageID,
			ReturnCode: sn.ReturnRejectedCongestion,
		})
		return
	}

	s.writePacket(conn, addr, &sn.SubAck{
		Flags:      sn.Flags{QoS: pkt.Flags.QoS, TopicIDType: pkt.Topic.Type},
		TopicID:    topicID,
		MessageID:  pkt.MessageID,
		ReturnCode: sn.ReturnAccepted,
	})
}

func (s *Server) handleUnsubscribe(conn net.PacketConn, addr net.Addr, pkt *sn.Unsubscribe) {
	sess := s.getSession(addr)
	if sess != nil && pkt.Topic.Type == sn.TopicIDTypeNormal {
		s.mu.Lock()
		sess.unregisterTopic(pkt.Topic.Name)
		sess.touch(s.now())
		s.mu.Unlock()
	}
	s.writePacket(conn, addr, &sn.UnsubAck{MessageID: pkt.MessageID})
}

func (s *Server) handlePublish(conn net.PacketConn, addr net.Addr, pkt *sn.Publish) {
	if pkt.Flags.QoS <= 0 {
		return
	}
	if pkt.Flags.TopicIDType != sn.TopicIDTypeNormal {
		s.writePacket(conn, addr, &sn.PubAck{
			TopicID:    pkt.TopicID,
			MessageID:  pkt.MessageID,
			ReturnCode: sn.ReturnRejectedNotSupported,
		})
		return
	}

	sess := s.getSession(addr)
	if sess == nil {
		s.writePacket(conn, addr, &sn.PubAck{
			TopicID:    pkt.TopicID,
			MessageID:  pkt.MessageID,
			ReturnCode: sn.ReturnRejectedNotSupported,
		})
		return
	}

	s.mu.Lock()
	_, ok := sess.topicName(pkt.TopicID)
	sess.touch(s.now())
	s.mu.Unlock()

	if !ok {
		s.writePacket(conn, addr, &sn.PubAck{
			TopicID:    pkt.TopicID,
			MessageID:  pkt.MessageID,
			ReturnCode: sn.ReturnRejectedInvalidTopicID,
		})
		return
	}

	s.writePacket(conn, addr, &sn.PubAck{
		TopicID:    pkt.TopicID,
		MessageID:  pkt.MessageID,
		ReturnCode: sn.ReturnRejectedNotSupported,
	})
}

func (s *Server) getSession(addr net.Addr) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[addr.String()]
}

func (s *Server) pruneExpired() {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, sess := range s.sessions {
		if sess.expired(now) {
			delete(s.sessions, key)
		}
	}
}

func (s *Server) writePacket(conn net.PacketConn, addr net.Addr, pkt sn.ControlPacket) {
	if pkt == nil {
		return
	}
	if _, err := conn.WriteTo(pkt.Encode(), addr); err != nil {
		s.logger.Warn("mqttsn_write_failed",
			slog.String("remote", addr.String()),
			slog.String("type", sn.PacketNames[pkt.Type()]),
			slog.String("error", err.Error()))
	}
}
