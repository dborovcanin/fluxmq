// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package mqttsn

import (
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/absmach/fluxmq/mqtt/packets/sn"
	"github.com/stretchr/testify/require"
)

type testAddr string

func (a testAddr) Network() string { return "udp" }
func (a testAddr) String() string  { return string(a) }

type writtenPacket struct {
	addr net.Addr
	data []byte
}

type recordingPacketConn struct {
	writes []writtenPacket
}

func (c *recordingPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, errors.New("not implemented")
}

func (c *recordingPacketConn) WriteTo(data []byte, addr net.Addr) (int, error) {
	cp := append([]byte(nil), data...)
	c.writes = append(c.writes, writtenPacket{addr: addr, data: cp})
	return len(data), nil
}

func (c *recordingPacketConn) Close() error                     { return nil }
func (c *recordingPacketConn) LocalAddr() net.Addr              { return testAddr("local") }
func (c *recordingPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *recordingPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *recordingPacketConn) SetWriteDeadline(time.Time) error { return nil }

func newTestServer() *Server {
	s := New(Config{Address: ":1884"}, slog.Default())
	now := time.Unix(100, 0)
	s.now = func() time.Time { return now }
	return s
}

func decodeLast[T sn.ControlPacket](t *testing.T, conn *recordingPacketConn) T {
	t.Helper()
	require.NotEmpty(t, conn.writes)
	pkt, err := sn.DecodePacket(conn.writes[len(conn.writes)-1].data)
	require.NoError(t, err)
	typed, ok := pkt.(T)
	require.Truef(t, ok, "decoded packet type %T", pkt)
	return typed
}

func TestNewDefaults(t *testing.T) {
	server := New(Config{}, nil)

	require.NotNil(t, server)
	require.Equal(t, 30*time.Second, server.config.ShutdownTimeout)
	require.Equal(t, defaultMaxPacketSize, server.config.MaxPacketSize)
	require.NotNil(t, server.logger)
}

func TestConnectPingDisconnect(t *testing.T) {
	server := newTestServer()
	conn := &recordingPacketConn{}
	addr := testAddr("client:1000")

	server.handleDatagram(conn, addr, (&sn.Connect{
		Flags:      sn.Flags{CleanSession: true},
		ProtocolID: sn.ProtocolID,
		Duration:   60,
		ClientID:   "sensor-1",
	}).Encode())

	connack := decodeLast[*sn.ConnAck](t, conn)
	require.Equal(t, sn.ReturnAccepted, connack.ReturnCode)
	require.Contains(t, server.sessions, addr.String())
	require.Equal(t, "sensor-1", server.sessions[addr.String()].ClientID)

	server.handleDatagram(conn, addr, (&sn.PingReq{}).Encode())
	decodeLast[*sn.PingResp](t, conn)

	server.handleDatagram(conn, addr, (&sn.Disconnect{}).Encode())
	require.NotContains(t, server.sessions, addr.String())
}

func TestConnectRejectsInvalidProtocol(t *testing.T) {
	server := newTestServer()
	conn := &recordingPacketConn{}
	addr := testAddr("client:1000")

	server.handleDatagram(conn, addr, (&sn.Connect{
		Flags:      sn.Flags{CleanSession: true},
		ProtocolID: 0x7F,
		Duration:   60,
		ClientID:   "sensor-1",
	}).Encode())

	connack := decodeLast[*sn.ConnAck](t, conn)
	require.Equal(t, sn.ReturnRejectedNotSupported, connack.ReturnCode)
	require.NotContains(t, server.sessions, addr.String())
}

func TestRegisterSubscribeUnsubscribe(t *testing.T) {
	server := newTestServer()
	conn := &recordingPacketConn{}
	addr := testAddr("client:1000")

	server.handleDatagram(conn, addr, (&sn.Connect{
		Flags:      sn.Flags{CleanSession: true},
		ProtocolID: sn.ProtocolID,
		Duration:   60,
		ClientID:   "sensor-1",
	}).Encode())

	server.handleDatagram(conn, addr, (&sn.Register{
		MessageID: 1,
		TopicName: "sensors/temp",
	}).Encode())
	regack := decodeLast[*sn.RegAck](t, conn)
	require.Equal(t, sn.ReturnAccepted, regack.ReturnCode)
	require.Equal(t, uint16(1), regack.TopicID)
	require.Equal(t, uint16(1), regack.MessageID)

	server.handleDatagram(conn, addr, (&sn.Subscribe{
		Flags:     sn.Flags{QoS: 1},
		MessageID: 2,
		Topic:     sn.TopicName("sensors/#"),
	}).Encode())
	suback := decodeLast[*sn.SubAck](t, conn)
	require.Equal(t, sn.ReturnAccepted, suback.ReturnCode)
	require.Equal(t, uint16(2), suback.TopicID)
	require.Equal(t, uint16(2), suback.MessageID)

	server.handleDatagram(conn, addr, (&sn.Unsubscribe{
		MessageID: 3,
		Topic:     sn.TopicName("sensors/#"),
	}).Encode())
	unsuback := decodeLast[*sn.UnsubAck](t, conn)
	require.Equal(t, uint16(3), unsuback.MessageID)

	_, ok := server.sessions[addr.String()].topicIDs["sensors/#"]
	require.False(t, ok)
}

func TestPublishWithoutBrokerIsRejectedAfterTopicValidation(t *testing.T) {
	server := newTestServer()
	conn := &recordingPacketConn{}
	addr := testAddr("client:1000")

	server.handleDatagram(conn, addr, (&sn.Connect{
		Flags:      sn.Flags{CleanSession: true},
		ProtocolID: sn.ProtocolID,
		Duration:   60,
		ClientID:   "sensor-1",
	}).Encode())
	server.handleDatagram(conn, addr, (&sn.Register{
		MessageID: 1,
		TopicName: "sensors/temp",
	}).Encode())

	server.handleDatagram(conn, addr, (&sn.Publish{
		Flags:     sn.Flags{QoS: 1, TopicIDType: sn.TopicIDTypeNormal},
		TopicID:   1,
		MessageID: 2,
		Data:      []byte("22.5"),
	}).Encode())

	puback := decodeLast[*sn.PubAck](t, conn)
	require.Equal(t, sn.ReturnRejectedNotSupported, puback.ReturnCode)
	require.Equal(t, uint16(1), puback.TopicID)
	require.Equal(t, uint16(2), puback.MessageID)
}

func TestPublishRejectsUnknownTopicID(t *testing.T) {
	server := newTestServer()
	conn := &recordingPacketConn{}
	addr := testAddr("client:1000")

	server.handleDatagram(conn, addr, (&sn.Connect{
		Flags:      sn.Flags{CleanSession: true},
		ProtocolID: sn.ProtocolID,
		Duration:   60,
		ClientID:   "sensor-1",
	}).Encode())
	server.handleDatagram(conn, addr, (&sn.Publish{
		Flags:     sn.Flags{QoS: 1, TopicIDType: sn.TopicIDTypeNormal},
		TopicID:   99,
		MessageID: 2,
		Data:      []byte("22.5"),
	}).Encode())

	puback := decodeLast[*sn.PubAck](t, conn)
	require.Equal(t, sn.ReturnRejectedInvalidTopicID, puback.ReturnCode)
}

func TestPublishRejectsPredefinedTopicUntilConfigured(t *testing.T) {
	server := newTestServer()
	conn := &recordingPacketConn{}
	addr := testAddr("client:1000")

	server.handleDatagram(conn, addr, (&sn.Connect{
		Flags:      sn.Flags{CleanSession: true},
		ProtocolID: sn.ProtocolID,
		Duration:   60,
		ClientID:   "sensor-1",
	}).Encode())
	server.handleDatagram(conn, addr, (&sn.Publish{
		Flags:     sn.Flags{QoS: 1, TopicIDType: sn.TopicIDTypePredefined},
		TopicID:   1,
		MessageID: 2,
		Data:      []byte("22.5"),
	}).Encode())

	puback := decodeLast[*sn.PubAck](t, conn)
	require.Equal(t, sn.ReturnRejectedNotSupported, puback.ReturnCode)
}

func TestSessionExpiry(t *testing.T) {
	server := New(Config{}, slog.Default())
	now := time.Unix(100, 0)
	server.now = func() time.Time { return now }

	conn := &recordingPacketConn{}
	addr := testAddr("client:1000")

	server.handleDatagram(conn, addr, (&sn.Connect{
		Flags:      sn.Flags{CleanSession: true},
		ProtocolID: sn.ProtocolID,
		Duration:   10,
		ClientID:   "sensor-1",
	}).Encode())
	require.Contains(t, server.sessions, addr.String())

	now = now.Add(16 * time.Second)
	server.handleDatagram(conn, testAddr("client:1001"), (&sn.PingReq{}).Encode())
	require.NotContains(t, server.sessions, addr.String())
}
