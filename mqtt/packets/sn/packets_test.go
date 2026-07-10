// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package sn_test

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/absmach/fluxmq/mqtt/packets/sn"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	sleep := uint16(30)
	tests := []sn.ControlPacket{
		&sn.Advertise{GatewayID: 1, Duration: 60},
		&sn.SearchGW{Radius: 2},
		&sn.GWInfo{GatewayID: 1, GatewayAddress: []byte{0xAA, 0xBB}},
		&sn.Connect{Flags: sn.Flags{CleanSession: true, Will: true}, ProtocolID: sn.ProtocolID, Duration: 60, ClientID: "sensor-1"},
		&sn.ConnAck{ReturnCode: sn.ReturnAccepted},
		&sn.WillTopicReq{},
		&sn.WillTopic{Flags: sn.Flags{QoS: 1, Retain: true}, Topic: "last/will"},
		&sn.WillMsgReq{},
		&sn.WillMsg{Message: []byte("offline")},
		&sn.Register{TopicID: 7, MessageID: 9, TopicName: "sensors/temp"},
		&sn.RegAck{TopicID: 7, MessageID: 9, ReturnCode: sn.ReturnAccepted},
		&sn.Publish{Flags: sn.Flags{QoS: 1, Retain: true, TopicIDType: sn.TopicIDTypeNormal}, TopicID: 7, MessageID: 10, Data: []byte("22.5")},
		&sn.PubAck{TopicID: 7, MessageID: 10, ReturnCode: sn.ReturnAccepted},
		&sn.PubRec{MessageID: 10},
		&sn.PubRel{MessageID: 10},
		&sn.PubComp{MessageID: 10},
		&sn.Subscribe{Flags: sn.Flags{QoS: 1}, MessageID: 11, Topic: sn.TopicName("sensors/#")},
		&sn.Subscribe{Flags: sn.Flags{QoS: 0, TopicIDType: sn.TopicIDTypePredefined}, MessageID: 12, Topic: sn.PredefinedTopic(3)},
		&sn.Subscribe{Flags: sn.Flags{QoS: sn.QoSMinusOne, TopicIDType: sn.TopicIDTypeShort}, MessageID: 13, Topic: sn.ShortTopicName("ab")},
		&sn.SubAck{Flags: sn.Flags{QoS: 1, TopicIDType: sn.TopicIDTypeNormal}, TopicID: 7, MessageID: 11, ReturnCode: sn.ReturnAccepted},
		&sn.Unsubscribe{MessageID: 14, Topic: sn.TopicName("sensors/#")},
		&sn.UnsubAck{MessageID: 14},
		&sn.PingReq{ClientID: "sensor-1"},
		&sn.PingResp{},
		&sn.Disconnect{},
		&sn.Disconnect{Duration: &sleep},
		&sn.WillTopicUpd{Flags: sn.Flags{QoS: 1}, Topic: "will/new"},
		&sn.WillTopicResp{ReturnCode: sn.ReturnAccepted},
		&sn.WillMsgUpd{Message: []byte("new will")},
		&sn.WillMsgResp{ReturnCode: sn.ReturnAccepted},
	}

	for _, tc := range tests {
		t.Run(reflect.TypeOf(tc).Elem().Name(), func(t *testing.T) {
			encoded := tc.Encode()
			require.NotEmpty(t, encoded)

			decoded, err := sn.DecodePacket(encoded)
			require.NoError(t, err)
			require.Equal(t, tc, decoded)

			fromReader, err := sn.ReadPacket(bytes.NewReader(encoded))
			require.NoError(t, err)
			require.Equal(t, tc, fromReader)

			var buf bytes.Buffer
			require.NoError(t, tc.Pack(&buf))
			require.Equal(t, encoded, buf.Bytes())
		})
	}
}

func TestKnownCONNECTEncoding(t *testing.T) {
	pkt := &sn.Connect{
		Flags:      sn.Flags{CleanSession: true},
		ProtocolID: sn.ProtocolID,
		Duration:   60,
		ClientID:   "dev1",
	}

	require.Equal(t, []byte{
		0x0A, sn.ConnectType,
		0x04, sn.ProtocolID,
		0x00, 0x3C,
		'd', 'e', 'v', '1',
	}, pkt.Encode())
}

func TestKnownPUBLISHEncoding(t *testing.T) {
	pkt := &sn.Publish{
		Flags:     sn.Flags{QoS: 1, Retain: true, TopicIDType: sn.TopicIDTypePredefined},
		TopicID:   0x0102,
		MessageID: 0x0304,
		Data:      []byte("x"),
	}

	require.Equal(t, []byte{
		0x08, sn.PublishType,
		0x31,
		0x01, 0x02,
		0x03, 0x04,
		'x',
	}, pkt.Encode())
}

func TestExtendedLengthHeader(t *testing.T) {
	payload := bytes.Repeat([]byte{0x42}, 260)
	pkt := &sn.Publish{
		Flags:     sn.Flags{QoS: 0, TopicIDType: sn.TopicIDTypeNormal},
		TopicID:   1,
		MessageID: 0,
		Data:      payload,
	}

	encoded := pkt.Encode()
	require.Len(t, encoded, 269)
	require.Equal(t, byte(0x01), encoded[0])
	require.Equal(t, byte(0x01), encoded[1])
	require.Equal(t, byte(0x0D), encoded[2])
	require.Equal(t, sn.PublishType, encoded[3])

	decoded, err := sn.DecodePacket(encoded)
	require.NoError(t, err)
	require.Equal(t, pkt, decoded)
}

func TestEncapsulatedDecodePacket(t *testing.T) {
	nested := (&sn.PingResp{}).Encode()
	pkt := &sn.Encapsulated{
		Ctrl:           0x00,
		WirelessNodeID: []byte{0xAA, 0xBB},
		Message:        nested,
	}

	encoded := pkt.Encode()
	require.Equal(t, []byte{0x05, sn.EncapsulatedType, 0x00, 0xAA, 0xBB, 0x02, sn.PingRespType}, encoded)

	decoded, err := sn.DecodePacket(encoded)
	require.NoError(t, err)
	require.Equal(t, pkt, decoded)
}

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		err  error
	}{
		{
			name: "short header",
			data: []byte{0x02},
			err:  io.ErrUnexpectedEOF,
		},
		{
			name: "invalid length",
			data: []byte{0x01, 0x00, 0x03, sn.PingRespType},
			err:  sn.ErrInvalidLength,
		},
		{
			name: "unsupported type",
			data: []byte{0x02, 0x03},
			err:  sn.ErrUnsupportedPacket,
		},
		{
			name: "trailing bytes",
			data: []byte{0x02, sn.PingRespType, 0x00},
			err:  sn.ErrInvalidLength,
		},
		{
			name: "truncated packet",
			data: []byte{0x05, sn.PublishType, 0x00},
			err:  sn.ErrInvalidLength,
		},
		{
			name: "reserved topic id type",
			data: []byte{0x05, sn.SubscribeType, 0x03, 0x00, 0x01},
			err:  sn.ErrInvalidTopicIDType,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sn.DecodePacket(tc.data)
			require.ErrorIs(t, err, tc.err)
		})
	}
}
