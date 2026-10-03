package codec

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
)

type Message struct {
	Type    MessageType
	Payload []byte
}

type MessageType uint8

const (
	MsgPing MessageType = iota
	MsgElection
	MsgHeartbeat
)

func Encode(msg Message) ([]byte, error) {
	length := uint32(1 + len(msg.Payload))

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, length)
	buf.WriteByte(byte(msg.Type))
	buf.Write(msg.Payload)

	return buf.Bytes(), nil
}

func Decode(conn net.Conn) (*Message, error) {
	var length uint32

	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}

	return &Message{
		Type:    MessageType(data[0]),
		Payload: data[1:],
	}, nil
}
