// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package sn

import (
	"fmt"
	"io"
)

type Advertise struct {
	GatewayID byte
	Duration  uint16
}

func (p *Advertise) Type() byte             { return AdvertiseType }
func (p *Advertise) Release()               {}
func (p *Advertise) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *Advertise) String() string {
	return packetString(p.Type(), fmt.Sprintf("gateway_id=%d duration=%d", p.GatewayID, p.Duration))
}
func (p *Advertise) Encode() []byte {
	body := appendUint16([]byte{p.GatewayID}, p.Duration)
	return encodePacket(p.Type(), body)
}
func (p *Advertise) Unpack(r io.Reader) error {
	b, err := readByte(r)
	if err != nil {
		return err
	}
	duration, err := readUint16(r)
	if err != nil {
		return err
	}
	if err := requireNoBytes(r); err != nil {
		return err
	}
	p.GatewayID = b
	p.Duration = duration
	return nil
}

type SearchGW struct {
	Radius byte
}

func (p *SearchGW) Type() byte             { return SearchGWType }
func (p *SearchGW) Release()               {}
func (p *SearchGW) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *SearchGW) String() string {
	return packetString(p.Type(), fmt.Sprintf("radius=%d", p.Radius))
}
func (p *SearchGW) Encode() []byte { return encodePacket(p.Type(), []byte{p.Radius}) }
func (p *SearchGW) Unpack(r io.Reader) error {
	b, err := readByte(r)
	if err != nil {
		return err
	}
	if err := requireNoBytes(r); err != nil {
		return err
	}
	p.Radius = b
	return nil
}

type GWInfo struct {
	GatewayID      byte
	GatewayAddress []byte
}

func (p *GWInfo) Type() byte             { return GWInfoType }
func (p *GWInfo) Release()               {}
func (p *GWInfo) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *GWInfo) String() string {
	return packetString(p.Type(), fmt.Sprintf("gateway_id=%d gateway_address=%x", p.GatewayID, p.GatewayAddress))
}
func (p *GWInfo) Encode() []byte {
	body := []byte{p.GatewayID}
	body = append(body, p.GatewayAddress...)
	return encodePacket(p.Type(), body)
}
func (p *GWInfo) Unpack(r io.Reader) error {
	b, err := readByte(r)
	if err != nil {
		return err
	}
	addr, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	p.GatewayID = b
	p.GatewayAddress = addr
	return nil
}

type Connect struct {
	Flags      Flags
	ProtocolID byte
	Duration   uint16
	ClientID   string
}

func (p *Connect) Type() byte             { return ConnectType }
func (p *Connect) Release()               {}
func (p *Connect) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *Connect) String() string {
	return packetString(p.Type(), fmt.Sprintf("flags=(%s) protocol_id=%d duration=%d client_id=%q",
		p.Flags, p.ProtocolID, p.Duration, p.ClientID))
}
func (p *Connect) Encode() []byte {
	body := []byte{p.Flags.encode(), p.ProtocolID}
	body = appendUint16(body, p.Duration)
	body = append(body, p.ClientID...)
	return encodePacket(p.Type(), body)
}
func (p *Connect) Unpack(r io.Reader) error {
	flags, err := readByte(r)
	if err != nil {
		return err
	}
	protocolID, err := readByte(r)
	if err != nil {
		return err
	}
	duration, err := readUint16(r)
	if err != nil {
		return err
	}
	clientID, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	p.Flags = decodeFlags(flags)
	p.ProtocolID = protocolID
	p.Duration = duration
	p.ClientID = string(clientID)
	return nil
}

type ConnAck struct {
	ReturnCode byte
}

func (p *ConnAck) Type() byte             { return ConnAckType }
func (p *ConnAck) Release()               {}
func (p *ConnAck) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *ConnAck) String() string {
	return packetString(p.Type(), fmt.Sprintf("return_code=%d", p.ReturnCode))
}
func (p *ConnAck) Encode() []byte { return encodePacket(p.Type(), []byte{p.ReturnCode}) }
func (p *ConnAck) Unpack(r io.Reader) error {
	b, err := readByte(r)
	if err != nil {
		return err
	}
	if err := requireNoBytes(r); err != nil {
		return err
	}
	p.ReturnCode = b
	return nil
}

type WillTopicReq struct{}

func (p *WillTopicReq) Type() byte               { return WillTopicReqType }
func (p *WillTopicReq) Release()                 {}
func (p *WillTopicReq) Pack(w io.Writer) error   { return pack(w, p.Encode()) }
func (p *WillTopicReq) String() string           { return packetString(p.Type(), "") }
func (p *WillTopicReq) Encode() []byte           { return encodePacket(p.Type(), nil) }
func (p *WillTopicReq) Unpack(r io.Reader) error { return requireNoBytes(r) }

type WillTopic struct {
	Flags Flags
	Topic string
}

func (p *WillTopic) Type() byte             { return WillTopicType }
func (p *WillTopic) Release()               {}
func (p *WillTopic) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *WillTopic) String() string {
	return packetString(p.Type(), fmt.Sprintf("flags=(%s) topic=%q", p.Flags, p.Topic))
}
func (p *WillTopic) Encode() []byte {
	if p.Topic == "" {
		return encodePacket(p.Type(), nil)
	}
	body := []byte{p.Flags.encode()}
	body = append(body, p.Topic...)
	return encodePacket(p.Type(), body)
}
func (p *WillTopic) Unpack(r io.Reader) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		p.Flags = Flags{}
		p.Topic = ""
		return nil
	}
	p.Flags = decodeFlags(body[0])
	p.Topic = string(body[1:])
	return nil
}

type WillMsgReq struct{}

func (p *WillMsgReq) Type() byte               { return WillMsgReqType }
func (p *WillMsgReq) Release()                 {}
func (p *WillMsgReq) Pack(w io.Writer) error   { return pack(w, p.Encode()) }
func (p *WillMsgReq) String() string           { return packetString(p.Type(), "") }
func (p *WillMsgReq) Encode() []byte           { return encodePacket(p.Type(), nil) }
func (p *WillMsgReq) Unpack(r io.Reader) error { return requireNoBytes(r) }

type WillMsg struct {
	Message []byte
}

func (p *WillMsg) Type() byte             { return WillMsgType }
func (p *WillMsg) Release()               {}
func (p *WillMsg) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *WillMsg) String() string {
	return packetString(p.Type(), fmt.Sprintf("message_len=%d", len(p.Message)))
}
func (p *WillMsg) Encode() []byte { return encodePacket(p.Type(), p.Message) }
func (p *WillMsg) Unpack(r io.Reader) error {
	msg, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	p.Message = msg
	return nil
}

type Register struct {
	TopicID   uint16
	MessageID uint16
	TopicName string
}

func (p *Register) Type() byte             { return RegisterType }
func (p *Register) Release()               {}
func (p *Register) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *Register) String() string {
	return packetString(p.Type(), fmt.Sprintf("topic_id=%d message_id=%d topic_name=%q", p.TopicID, p.MessageID, p.TopicName))
}
func (p *Register) Encode() []byte {
	body := appendUint16(nil, p.TopicID)
	body = appendUint16(body, p.MessageID)
	body = append(body, p.TopicName...)
	return encodePacket(p.Type(), body)
}
func (p *Register) Unpack(r io.Reader) error {
	topicID, err := readUint16(r)
	if err != nil {
		return err
	}
	msgID, err := readUint16(r)
	if err != nil {
		return err
	}
	topicName, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	p.TopicID = topicID
	p.MessageID = msgID
	p.TopicName = string(topicName)
	return nil
}

type RegAck struct {
	TopicID    uint16
	MessageID  uint16
	ReturnCode byte
}

func (p *RegAck) Type() byte             { return RegAckType }
func (p *RegAck) Release()               {}
func (p *RegAck) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *RegAck) String() string {
	return packetString(p.Type(), fmt.Sprintf("topic_id=%d message_id=%d return_code=%d", p.TopicID, p.MessageID, p.ReturnCode))
}
func (p *RegAck) Encode() []byte {
	body := appendUint16(nil, p.TopicID)
	body = appendUint16(body, p.MessageID)
	body = append(body, p.ReturnCode)
	return encodePacket(p.Type(), body)
}
func (p *RegAck) Unpack(r io.Reader) error {
	topicID, err := readUint16(r)
	if err != nil {
		return err
	}
	msgID, err := readUint16(r)
	if err != nil {
		return err
	}
	rc, err := readByte(r)
	if err != nil {
		return err
	}
	if err := requireNoBytes(r); err != nil {
		return err
	}
	p.TopicID = topicID
	p.MessageID = msgID
	p.ReturnCode = rc
	return nil
}

type Publish struct {
	Flags     Flags
	TopicID   uint16
	MessageID uint16
	Data      []byte
}

func (p *Publish) Type() byte             { return PublishType }
func (p *Publish) Release()               {}
func (p *Publish) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *Publish) String() string {
	return packetString(p.Type(), fmt.Sprintf("flags=(%s) topic_id=%d message_id=%d data_len=%d",
		p.Flags, p.TopicID, p.MessageID, len(p.Data)))
}
func (p *Publish) Encode() []byte {
	body := []byte{p.Flags.encode()}
	body = appendUint16(body, p.TopicID)
	body = appendUint16(body, p.MessageID)
	body = append(body, p.Data...)
	return encodePacket(p.Type(), body)
}
func (p *Publish) Unpack(r io.Reader) error {
	flags, err := readByte(r)
	if err != nil {
		return err
	}
	topicID, err := readUint16(r)
	if err != nil {
		return err
	}
	msgID, err := readUint16(r)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	p.Flags = decodeFlags(flags)
	p.TopicID = topicID
	p.MessageID = msgID
	p.Data = data
	return nil
}

type PubAck struct {
	TopicID    uint16
	MessageID  uint16
	ReturnCode byte
}

func (p *PubAck) Type() byte             { return PubAckType }
func (p *PubAck) Release()               {}
func (p *PubAck) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *PubAck) String() string {
	return packetString(p.Type(), fmt.Sprintf("topic_id=%d message_id=%d return_code=%d", p.TopicID, p.MessageID, p.ReturnCode))
}
func (p *PubAck) Encode() []byte {
	body := appendUint16(nil, p.TopicID)
	body = appendUint16(body, p.MessageID)
	body = append(body, p.ReturnCode)
	return encodePacket(p.Type(), body)
}
func (p *PubAck) Unpack(r io.Reader) error {
	topicID, err := readUint16(r)
	if err != nil {
		return err
	}
	msgID, err := readUint16(r)
	if err != nil {
		return err
	}
	rc, err := readByte(r)
	if err != nil {
		return err
	}
	if err := requireNoBytes(r); err != nil {
		return err
	}
	p.TopicID = topicID
	p.MessageID = msgID
	p.ReturnCode = rc
	return nil
}

type PubRec struct {
	MessageID uint16
}

func (p *PubRec) Type() byte             { return PubRecType }
func (p *PubRec) Release()               {}
func (p *PubRec) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *PubRec) String() string {
	return packetString(p.Type(), fmt.Sprintf("message_id=%d", p.MessageID))
}
func (p *PubRec) Encode() []byte { return encodePacket(p.Type(), appendUint16(nil, p.MessageID)) }
func (p *PubRec) Unpack(r io.Reader) error {
	return unpackMessageID(r, &p.MessageID)
}

type PubRel struct {
	MessageID uint16
}

func (p *PubRel) Type() byte             { return PubRelType }
func (p *PubRel) Release()               {}
func (p *PubRel) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *PubRel) String() string {
	return packetString(p.Type(), fmt.Sprintf("message_id=%d", p.MessageID))
}
func (p *PubRel) Encode() []byte { return encodePacket(p.Type(), appendUint16(nil, p.MessageID)) }
func (p *PubRel) Unpack(r io.Reader) error {
	return unpackMessageID(r, &p.MessageID)
}

type PubComp struct {
	MessageID uint16
}

func (p *PubComp) Type() byte             { return PubCompType }
func (p *PubComp) Release()               {}
func (p *PubComp) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *PubComp) String() string {
	return packetString(p.Type(), fmt.Sprintf("message_id=%d", p.MessageID))
}
func (p *PubComp) Encode() []byte { return encodePacket(p.Type(), appendUint16(nil, p.MessageID)) }
func (p *PubComp) Unpack(r io.Reader) error {
	return unpackMessageID(r, &p.MessageID)
}

type TopicSelector struct {
	Type      byte
	Name      string
	ID        uint16
	ShortName string
}

func TopicName(name string) TopicSelector {
	return TopicSelector{Type: TopicIDTypeNormal, Name: name}
}

func PredefinedTopic(id uint16) TopicSelector {
	return TopicSelector{Type: TopicIDTypePredefined, ID: id}
}

func ShortTopicName(name string) TopicSelector {
	return TopicSelector{Type: TopicIDTypeShort, ShortName: name}
}

func (t TopicSelector) encode(dst []byte) []byte {
	switch t.Type {
	case TopicIDTypeNormal:
		return append(dst, t.Name...)
	case TopicIDTypePredefined:
		return appendUint16(dst, t.ID)
	case TopicIDTypeShort:
		name := []byte(t.ShortName)
		if len(name) >= 2 {
			return append(dst, name[0], name[1])
		}
		if len(name) == 1 {
			return append(dst, name[0], 0)
		}
		return append(dst, 0, 0)
	default:
		return dst
	}
}

func decodeTopicSelector(topicType byte, data []byte) (TopicSelector, error) {
	switch topicType {
	case TopicIDTypeNormal:
		return TopicSelector{Type: topicType, Name: string(data)}, nil
	case TopicIDTypePredefined:
		if len(data) != 2 {
			return TopicSelector{}, ErrMalformedPacket
		}
		return TopicSelector{Type: topicType, ID: uint16(data[0])<<8 | uint16(data[1])}, nil
	case TopicIDTypeShort:
		if len(data) != 2 {
			return TopicSelector{}, ErrMalformedPacket
		}
		return TopicSelector{Type: topicType, ShortName: string(data)}, nil
	default:
		return TopicSelector{}, ErrInvalidTopicIDType
	}
}

type Subscribe struct {
	Flags     Flags
	MessageID uint16
	Topic     TopicSelector
}

func (p *Subscribe) Type() byte             { return SubscribeType }
func (p *Subscribe) Release()               {}
func (p *Subscribe) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *Subscribe) String() string {
	return packetString(p.Type(), fmt.Sprintf("flags=(%s) message_id=%d topic=%+v", p.Flags, p.MessageID, p.Topic))
}
func (p *Subscribe) Encode() []byte {
	flags := p.Flags
	flags.TopicIDType = p.Topic.Type
	body := []byte{flags.encode()}
	body = appendUint16(body, p.MessageID)
	body = p.Topic.encode(body)
	return encodePacket(p.Type(), body)
}
func (p *Subscribe) Unpack(r io.Reader) error {
	flags, err := readByte(r)
	if err != nil {
		return err
	}
	msgID, err := readUint16(r)
	if err != nil {
		return err
	}
	topic, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	decodedFlags := decodeFlags(flags)
	selector, err := decodeTopicSelector(decodedFlags.TopicIDType, topic)
	if err != nil {
		return err
	}
	p.Flags = decodedFlags
	p.MessageID = msgID
	p.Topic = selector
	return nil
}

type SubAck struct {
	Flags      Flags
	TopicID    uint16
	MessageID  uint16
	ReturnCode byte
}

func (p *SubAck) Type() byte             { return SubAckType }
func (p *SubAck) Release()               {}
func (p *SubAck) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *SubAck) String() string {
	return packetString(p.Type(), fmt.Sprintf("flags=(%s) topic_id=%d message_id=%d return_code=%d",
		p.Flags, p.TopicID, p.MessageID, p.ReturnCode))
}
func (p *SubAck) Encode() []byte {
	body := []byte{p.Flags.encode()}
	body = appendUint16(body, p.TopicID)
	body = appendUint16(body, p.MessageID)
	body = append(body, p.ReturnCode)
	return encodePacket(p.Type(), body)
}
func (p *SubAck) Unpack(r io.Reader) error {
	flags, err := readByte(r)
	if err != nil {
		return err
	}
	topicID, err := readUint16(r)
	if err != nil {
		return err
	}
	msgID, err := readUint16(r)
	if err != nil {
		return err
	}
	rc, err := readByte(r)
	if err != nil {
		return err
	}
	if err := requireNoBytes(r); err != nil {
		return err
	}
	p.Flags = decodeFlags(flags)
	p.TopicID = topicID
	p.MessageID = msgID
	p.ReturnCode = rc
	return nil
}

type Unsubscribe struct {
	Flags     Flags
	MessageID uint16
	Topic     TopicSelector
}

func (p *Unsubscribe) Type() byte             { return UnsubscribeType }
func (p *Unsubscribe) Release()               {}
func (p *Unsubscribe) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *Unsubscribe) String() string {
	return packetString(p.Type(), fmt.Sprintf("flags=(%s) message_id=%d topic=%+v", p.Flags, p.MessageID, p.Topic))
}
func (p *Unsubscribe) Encode() []byte {
	flags := p.Flags
	flags.TopicIDType = p.Topic.Type
	body := []byte{flags.encode()}
	body = appendUint16(body, p.MessageID)
	body = p.Topic.encode(body)
	return encodePacket(p.Type(), body)
}
func (p *Unsubscribe) Unpack(r io.Reader) error {
	flags, err := readByte(r)
	if err != nil {
		return err
	}
	msgID, err := readUint16(r)
	if err != nil {
		return err
	}
	topic, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	decodedFlags := decodeFlags(flags)
	selector, err := decodeTopicSelector(decodedFlags.TopicIDType, topic)
	if err != nil {
		return err
	}
	p.Flags = decodedFlags
	p.MessageID = msgID
	p.Topic = selector
	return nil
}

type UnsubAck struct {
	MessageID uint16
}

func (p *UnsubAck) Type() byte             { return UnsubAckType }
func (p *UnsubAck) Release()               {}
func (p *UnsubAck) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *UnsubAck) String() string {
	return packetString(p.Type(), fmt.Sprintf("message_id=%d", p.MessageID))
}
func (p *UnsubAck) Encode() []byte { return encodePacket(p.Type(), appendUint16(nil, p.MessageID)) }
func (p *UnsubAck) Unpack(r io.Reader) error {
	return unpackMessageID(r, &p.MessageID)
}

type PingReq struct {
	ClientID string
}

func (p *PingReq) Type() byte             { return PingReqType }
func (p *PingReq) Release()               {}
func (p *PingReq) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *PingReq) String() string {
	return packetString(p.Type(), fmt.Sprintf("client_id=%q", p.ClientID))
}
func (p *PingReq) Encode() []byte { return encodePacket(p.Type(), []byte(p.ClientID)) }
func (p *PingReq) Unpack(r io.Reader) error {
	clientID, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	p.ClientID = string(clientID)
	return nil
}

type PingResp struct{}

func (p *PingResp) Type() byte               { return PingRespType }
func (p *PingResp) Release()                 {}
func (p *PingResp) Pack(w io.Writer) error   { return pack(w, p.Encode()) }
func (p *PingResp) String() string           { return packetString(p.Type(), "") }
func (p *PingResp) Encode() []byte           { return encodePacket(p.Type(), nil) }
func (p *PingResp) Unpack(r io.Reader) error { return requireNoBytes(r) }

type Disconnect struct {
	Duration *uint16
}

func (p *Disconnect) Type() byte             { return DisconnectType }
func (p *Disconnect) Release()               {}
func (p *Disconnect) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *Disconnect) String() string {
	if p.Duration == nil {
		return packetString(p.Type(), "duration=<nil>")
	}
	return packetString(p.Type(), fmt.Sprintf("duration=%d", *p.Duration))
}
func (p *Disconnect) Encode() []byte {
	if p.Duration == nil {
		return encodePacket(p.Type(), nil)
	}
	return encodePacket(p.Type(), appendUint16(nil, *p.Duration))
}
func (p *Disconnect) Unpack(r io.Reader) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	switch len(body) {
	case 0:
		p.Duration = nil
	case 2:
		duration := uint16(body[0])<<8 | uint16(body[1])
		p.Duration = &duration
	default:
		return ErrMalformedPacket
	}
	return nil
}

type WillTopicUpd struct {
	Flags Flags
	Topic string
}

func (p *WillTopicUpd) Type() byte             { return WillTopicUpdType }
func (p *WillTopicUpd) Release()               {}
func (p *WillTopicUpd) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *WillTopicUpd) String() string {
	return packetString(p.Type(), fmt.Sprintf("flags=(%s) topic=%q", p.Flags, p.Topic))
}
func (p *WillTopicUpd) Encode() []byte {
	if p.Topic == "" {
		return encodePacket(p.Type(), nil)
	}
	body := []byte{p.Flags.encode()}
	body = append(body, p.Topic...)
	return encodePacket(p.Type(), body)
}
func (p *WillTopicUpd) Unpack(r io.Reader) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		p.Flags = Flags{}
		p.Topic = ""
		return nil
	}
	p.Flags = decodeFlags(body[0])
	p.Topic = string(body[1:])
	return nil
}

type WillTopicResp struct {
	ReturnCode byte
}

func (p *WillTopicResp) Type() byte             { return WillTopicRespType }
func (p *WillTopicResp) Release()               {}
func (p *WillTopicResp) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *WillTopicResp) String() string {
	return packetString(p.Type(), fmt.Sprintf("return_code=%d", p.ReturnCode))
}
func (p *WillTopicResp) Encode() []byte { return encodePacket(p.Type(), []byte{p.ReturnCode}) }
func (p *WillTopicResp) Unpack(r io.Reader) error {
	return unpackReturnCode(r, &p.ReturnCode)
}

type WillMsgUpd struct {
	Message []byte
}

func (p *WillMsgUpd) Type() byte             { return WillMsgUpdType }
func (p *WillMsgUpd) Release()               {}
func (p *WillMsgUpd) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *WillMsgUpd) String() string {
	return packetString(p.Type(), fmt.Sprintf("message_len=%d", len(p.Message)))
}
func (p *WillMsgUpd) Encode() []byte { return encodePacket(p.Type(), p.Message) }
func (p *WillMsgUpd) Unpack(r io.Reader) error {
	msg, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	p.Message = msg
	return nil
}

type WillMsgResp struct {
	ReturnCode byte
}

func (p *WillMsgResp) Type() byte             { return WillMsgRespType }
func (p *WillMsgResp) Release()               {}
func (p *WillMsgResp) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *WillMsgResp) String() string {
	return packetString(p.Type(), fmt.Sprintf("return_code=%d", p.ReturnCode))
}
func (p *WillMsgResp) Encode() []byte { return encodePacket(p.Type(), []byte{p.ReturnCode}) }
func (p *WillMsgResp) Unpack(r io.Reader) error {
	return unpackReturnCode(r, &p.ReturnCode)
}

type Encapsulated struct {
	Ctrl           byte
	WirelessNodeID []byte
	Message        []byte
}

func (p *Encapsulated) Type() byte             { return EncapsulatedType }
func (p *Encapsulated) Release()               {}
func (p *Encapsulated) Pack(w io.Writer) error { return pack(w, p.Encode()) }
func (p *Encapsulated) String() string {
	return packetString(p.Type(), fmt.Sprintf("ctrl=0x%02x wireless_node_id=%x message_len=%d",
		p.Ctrl, p.WirelessNodeID, len(p.Message)))
}
func (p *Encapsulated) Encode() []byte {
	outerLen := 3 + len(p.WirelessNodeID)
	body := []byte{EncapsulatedType, p.Ctrl}
	body = append(body, p.WirelessNodeID...)
	out := []byte{byte(outerLen)}
	out = append(out, body...)
	out = append(out, p.Message...)
	return out
}
func (p *Encapsulated) Unpack(io.Reader) error {
	return ErrEncapsulatedUnpack
}
func (p *Encapsulated) unpackFromDatagram(data []byte, h header) error {
	if h.length < h.offset+1 {
		return ErrInvalidLength
	}
	if len(data) < h.length {
		return ErrInvalidLength
	}
	p.Ctrl = data[h.offset]
	p.WirelessNodeID = append(p.WirelessNodeID[:0], data[h.offset+1:h.length]...)
	p.Message = append(p.Message[:0], data[h.length:]...)
	return nil
}

func unpackMessageID(r io.Reader, dst *uint16) error {
	msgID, err := readUint16(r)
	if err != nil {
		return err
	}
	if err := requireNoBytes(r); err != nil {
		return err
	}
	*dst = msgID
	return nil
}

func unpackReturnCode(r io.Reader, dst *byte) error {
	rc, err := readByte(r)
	if err != nil {
		return err
	}
	if err := requireNoBytes(r); err != nil {
		return err
	}
	*dst = rc
	return nil
}
