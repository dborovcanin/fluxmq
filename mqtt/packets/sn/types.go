// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package sn

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

const (
	longLengthMarker = 0x01
	maxPacketLength  = 0xFFFF
)

// MQTT-SN 1.2 packet type constants.
const (
	AdvertiseType     byte = 0x00
	SearchGWType      byte = 0x01
	GWInfoType        byte = 0x02
	ConnectType       byte = 0x04
	ConnAckType       byte = 0x05
	WillTopicReqType  byte = 0x06
	WillTopicType     byte = 0x07
	WillMsgReqType    byte = 0x08
	WillMsgType       byte = 0x09
	RegisterType      byte = 0x0A
	RegAckType        byte = 0x0B
	PublishType       byte = 0x0C
	PubAckType        byte = 0x0D
	PubCompType       byte = 0x0E
	PubRecType        byte = 0x0F
	PubRelType        byte = 0x10
	SubscribeType     byte = 0x11
	SubAckType        byte = 0x12
	UnsubscribeType   byte = 0x13
	UnsubAckType      byte = 0x14
	PingReqType       byte = 0x16
	PingRespType      byte = 0x17
	DisconnectType    byte = 0x18
	WillTopicUpdType  byte = 0x1A
	WillTopicRespType byte = 0x1B
	WillMsgUpdType    byte = 0x1C
	WillMsgRespType   byte = 0x1D
	EncapsulatedType  byte = 0xFE
)

// MQTT-SN return codes.
const (
	ReturnAccepted               byte = 0x00
	ReturnRejectedCongestion     byte = 0x01
	ReturnRejectedInvalidTopicID byte = 0x02
	ReturnRejectedNotSupported   byte = 0x03
)

// MQTT-SN topic ID types encoded in the low two flag bits.
const (
	TopicIDTypeNormal     byte = 0x00
	TopicIDTypePredefined byte = 0x01
	TopicIDTypeShort      byte = 0x02
	TopicIDTypeReserved   byte = 0x03
)

// ProtocolID is the MQTT-SN 1.2 protocol identifier used in CONNECT packets.
const ProtocolID byte = 0x01

// QoSMinusOne is MQTT-SN's fire-and-forget QoS level. On the wire this is
// encoded as QoS flag bits 11.
const QoSMinusOne int8 = -1

var (
	ErrInvalidLength        = errors.New("invalid MQTT-SN packet length")
	ErrMalformedPacket      = errors.New("malformed MQTT-SN packet")
	ErrMessageTooLarge      = errors.New("MQTT-SN packet exceeds maximum length")
	ErrUnsupportedPacket    = errors.New("unsupported MQTT-SN packet type")
	ErrInvalidTopicIDType   = errors.New("invalid MQTT-SN topic id type")
	ErrInvalidFlags         = errors.New("invalid MQTT-SN flags")
	ErrEncapsulatedUnpack   = errors.New("encapsulated MQTT-SN packets require DecodePacket")
	ErrEncapsulatedReadRest = errors.New("encapsulated MQTT-SN packet cannot read nested message")
)

// PacketNames maps MQTT-SN packet types to their protocol names.
var PacketNames = map[byte]string{
	AdvertiseType:     "ADVERTISE",
	SearchGWType:      "SEARCHGW",
	GWInfoType:        "GWINFO",
	ConnectType:       "CONNECT",
	ConnAckType:       "CONNACK",
	WillTopicReqType:  "WILLTOPICREQ",
	WillTopicType:     "WILLTOPIC",
	WillMsgReqType:    "WILLMSGREQ",
	WillMsgType:       "WILLMSG",
	RegisterType:      "REGISTER",
	RegAckType:        "REGACK",
	PublishType:       "PUBLISH",
	PubAckType:        "PUBACK",
	PubCompType:       "PUBCOMP",
	PubRecType:        "PUBREC",
	PubRelType:        "PUBREL",
	SubscribeType:     "SUBSCRIBE",
	SubAckType:        "SUBACK",
	UnsubscribeType:   "UNSUBSCRIBE",
	UnsubAckType:      "UNSUBACK",
	PingReqType:       "PINGREQ",
	PingRespType:      "PINGRESP",
	DisconnectType:    "DISCONNECT",
	WillTopicUpdType:  "WILLTOPICUPD",
	WillTopicRespType: "WILLTOPICRESP",
	WillMsgUpdType:    "WILLMSGUPD",
	WillMsgRespType:   "WILLMSGRESP",
	EncapsulatedType:  "ENCAPSULATED",
}

// ControlPacket is the common interface for MQTT-SN packets.
type ControlPacket interface {
	Encode() []byte
	Pack(io.Writer) error
	Unpack(io.Reader) error
	Type() byte
	Release()
	String() string
}

// Flags represents MQTT-SN's single-byte flag field.
type Flags struct {
	Dup          bool
	QoS          int8
	Retain       bool
	Will         bool
	CleanSession bool
	TopicIDType  byte
}

func (f Flags) encode() byte {
	qos := byte(0)
	switch f.QoS {
	case QoSMinusOne:
		qos = 0x03
	case 0, 1, 2:
		qos = byte(f.QoS)
	}

	var b byte
	if f.Dup {
		b |= 0x80
	}
	b |= qos << 5
	if f.Retain {
		b |= 0x10
	}
	if f.Will {
		b |= 0x08
	}
	if f.CleanSession {
		b |= 0x04
	}
	b |= f.TopicIDType & 0x03
	return b
}

func decodeFlags(b byte) Flags {
	qosBits := (b >> 5) & 0x03
	qos := int8(qosBits)
	if qosBits == 0x03 {
		qos = QoSMinusOne
	}

	return Flags{
		Dup:          b&0x80 != 0,
		QoS:          qos,
		Retain:       b&0x10 != 0,
		Will:         b&0x08 != 0,
		CleanSession: b&0x04 != 0,
		TopicIDType:  b & 0x03,
	}
}

// Validate reports whether the flag field is representable by MQTT-SN.
func (f Flags) Validate() error {
	switch f.QoS {
	case QoSMinusOne, 0, 1, 2:
	default:
		return ErrInvalidFlags
	}
	if f.TopicIDType > TopicIDTypeReserved {
		return ErrInvalidTopicIDType
	}
	return nil
}

func (f Flags) String() string {
	return fmt.Sprintf("dup=%t qos=%d retain=%t will=%t clean_session=%t topic_id_type=%d",
		f.Dup, f.QoS, f.Retain, f.Will, f.CleanSession, f.TopicIDType)
}

type header struct {
	length    int
	packetTyp byte
	offset    int
}

// NewControlPacket creates a zero-valued MQTT-SN packet for a packet type.
func NewControlPacket(packetType byte) ControlPacket {
	switch packetType {
	case AdvertiseType:
		return &Advertise{}
	case SearchGWType:
		return &SearchGW{}
	case GWInfoType:
		return &GWInfo{}
	case ConnectType:
		return &Connect{}
	case ConnAckType:
		return &ConnAck{}
	case WillTopicReqType:
		return &WillTopicReq{}
	case WillTopicType:
		return &WillTopic{}
	case WillMsgReqType:
		return &WillMsgReq{}
	case WillMsgType:
		return &WillMsg{}
	case RegisterType:
		return &Register{}
	case RegAckType:
		return &RegAck{}
	case PublishType:
		return &Publish{}
	case PubAckType:
		return &PubAck{}
	case PubCompType:
		return &PubComp{}
	case PubRecType:
		return &PubRec{}
	case PubRelType:
		return &PubRel{}
	case SubscribeType:
		return &Subscribe{}
	case SubAckType:
		return &SubAck{}
	case UnsubscribeType:
		return &Unsubscribe{}
	case UnsubAckType:
		return &UnsubAck{}
	case PingReqType:
		return &PingReq{}
	case PingRespType:
		return &PingResp{}
	case DisconnectType:
		return &Disconnect{}
	case WillTopicUpdType:
		return &WillTopicUpd{}
	case WillTopicRespType:
		return &WillTopicResp{}
	case WillMsgUpdType:
		return &WillMsgUpd{}
	case WillMsgRespType:
		return &WillMsgResp{}
	case EncapsulatedType:
		return &Encapsulated{}
	default:
		return nil
	}
}

// DecodePacket decodes a complete MQTT-SN datagram.
func DecodePacket(data []byte) (ControlPacket, error) {
	h, err := decodeHeader(data)
	if err != nil {
		return nil, err
	}

	pkt := NewControlPacket(h.packetTyp)
	if pkt == nil {
		return nil, fmt.Errorf("%w: 0x%02x", ErrUnsupportedPacket, h.packetTyp)
	}

	if enc, ok := pkt.(*Encapsulated); ok {
		if err := enc.unpackFromDatagram(data, h); err != nil {
			return nil, err
		}
		return enc, nil
	}

	if len(data) < h.length {
		return nil, ErrInvalidLength
	}
	if len(data) != h.length {
		return nil, ErrInvalidLength
	}
	if err := pkt.Unpack(bytes.NewReader(data[h.offset:h.length])); err != nil {
		return nil, err
	}
	return pkt, nil
}

// ReadPacket reads a single MQTT-SN packet from r.
//
// MQTT-SN is normally transported as datagrams. DecodePacket is preferable when
// the transport already provides datagram boundaries.
func ReadPacket(r io.Reader) (ControlPacket, error) {
	first := make([]byte, 1)
	if _, err := io.ReadFull(r, first); err != nil {
		return nil, err
	}

	data := []byte{first[0]}
	var total int
	if first[0] == longLengthMarker {
		rest := make([]byte, 3)
		if _, err := io.ReadFull(r, rest); err != nil {
			return nil, err
		}
		data = append(data, rest...)
		total = int(rest[0])<<8 | int(rest[1])
	} else {
		rest := make([]byte, 1)
		if _, err := io.ReadFull(r, rest); err != nil {
			return nil, err
		}
		data = append(data, rest...)
		total = int(first[0])
	}

	h, err := decodeHeader(data)
	if err != nil {
		return nil, err
	}
	if total != h.length {
		return nil, ErrInvalidLength
	}

	remaining := h.length - len(data)
	if remaining < 0 {
		return nil, ErrInvalidLength
	}
	body := make([]byte, remaining)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	data = append(data, body...)

	if h.packetTyp == EncapsulatedType {
		nested, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrEncapsulatedReadRest, err)
		}
		data = append(data, nested...)
	}

	return DecodePacket(data)
}

func encodePacket(packetType byte, body []byte) []byte {
	total := len(body) + 2
	if total <= 0xFF {
		out := []byte{byte(total), packetType}
		return append(out, body...)
	}

	total = len(body) + 4
	if total > maxPacketLength {
		return nil
	}
	out := []byte{longLengthMarker, byte(total >> 8), byte(total), packetType}
	return append(out, body...)
}

func decodeHeader(data []byte) (header, error) {
	if len(data) < 2 {
		return header{}, io.ErrUnexpectedEOF
	}

	if data[0] == longLengthMarker {
		if len(data) < 4 {
			return header{}, io.ErrUnexpectedEOF
		}
		length := int(data[1])<<8 | int(data[2])
		if length < 4 {
			return header{}, ErrInvalidLength
		}
		return header{length: length, packetTyp: data[3], offset: 4}, nil
	}

	length := int(data[0])
	if length < 2 {
		return header{}, ErrInvalidLength
	}
	return header{length: length, packetTyp: data[1], offset: 2}, nil
}

func pack(w io.Writer, data []byte) error {
	_, err := w.Write(data)
	return err
}

func packetString(packetType byte, fields string) string {
	name := PacketNames[packetType]
	if name == "" {
		name = fmt.Sprintf("0x%02x", packetType)
	}
	if fields == "" {
		return name
	}
	return name + " " + fields
}

func readByte(r io.Reader) (byte, error) {
	var b [1]byte
	_, err := io.ReadFull(r, b[:])
	return b[0], err
}

func readUint16(r io.Reader) (uint16, error) {
	var b [2]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return uint16(b[0])<<8 | uint16(b[1]), nil
}

func appendUint16(dst []byte, v uint16) []byte {
	return append(dst, byte(v>>8), byte(v))
}

func requireNoBytes(r io.Reader) error {
	rest, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return ErrMalformedPacket
	}
	return nil
}
