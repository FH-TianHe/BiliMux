package protocol

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/gorilla/websocket"
)

const (
	HeaderLength     = 16
	VersionPlain     = 0
	VersionZlib      = 2
	VersionBrotli    = 3
	OpHeartbeat      = 2
	OpHeartbeatReply = 3
	OpMessage        = 5
	OpAuth           = 7
	OpAuthReply      = 8
)

type PacketHeader struct {
	PacketLength uint32
	HeaderLength uint16
	Version      uint16
	Operation    uint32
	SequenceID   uint32
}

type AuthBody struct {
	UID      int    `json:"uid"`
	RoomID   int    `json:"roomid"`
	ProtoVer int    `json:"protover"`
	Buvid    string `json:"buvid"`
	Platform string `json:"platform"`
	Type     int    `json:"type"`
	Key      string `json:"key"`
}

// 解析数据包
func ParsePacket(data []byte) (header PacketHeader, body []byte, err error) {
	if len(data) < HeaderLength {
		return PacketHeader{}, nil, fmt.Errorf("数据包长度不足")
	}

	header.PacketLength = binary.BigEndian.Uint32(data[0:4])
	header.HeaderLength = binary.BigEndian.Uint16(data[4:6])
	header.Version = binary.BigEndian.Uint16(data[6:8])
	header.Operation = binary.BigEndian.Uint32(data[8:12])
	header.SequenceID = binary.BigEndian.Uint32(data[12:16])

	if int(header.PacketLength) > len(data) {
		return PacketHeader{}, nil, fmt.Errorf("数据包长度错误")
	}

	body = data[header.HeaderLength:header.PacketLength]

	// 处理压缩数据
	switch header.Version {
	case VersionZlib:
		r, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			return PacketHeader{}, nil, err
		}
		defer r.Close()
		decompressed, err := io.ReadAll(r)
		if err != nil {
			return PacketHeader{}, nil, err
		}
		body = decompressed
	case VersionBrotli:
		// TODO: 添加brotli解压支持
	}

	return header, body, nil
}

// 创建认证包
func CreateAuthPacket(roomID int, token, buvid3 string) ([]byte, error) {
	authBody := AuthBody{
		UID:      0,
		RoomID:   roomID,
		ProtoVer: 3,
		Buvid:    buvid3,
		Platform: "web",
		Type:     2,
		Key:      token,
	}

	body, err := json.Marshal(authBody)
	if err != nil {
		return nil, err
	}

	header := PacketHeader{
		PacketLength: uint32(HeaderLength + len(body)),
		HeaderLength: HeaderLength,
		Version:      1,
		Operation:    OpAuth,
		SequenceID:   1,
	}

	packet := make([]byte, header.PacketLength)
	binary.BigEndian.PutUint32(packet[0:4], header.PacketLength)
	binary.BigEndian.PutUint16(packet[4:6], header.HeaderLength)
	binary.BigEndian.PutUint16(packet[6:8], header.Version)
	binary.BigEndian.PutUint32(packet[8:12], header.Operation)
	binary.BigEndian.PutUint32(packet[12:16], header.SequenceID)
	copy(packet[16:], body)

	return packet, nil
}

// 创建心跳包
func CreateHeartbeatPacket() []byte {
	header := PacketHeader{
		PacketLength: HeaderLength,
		HeaderLength: HeaderLength,
		Version:      1,
		Operation:    OpHeartbeat,
		SequenceID:   1,
	}

	packet := make([]byte, header.PacketLength)
	binary.BigEndian.PutUint32(packet[0:4], header.PacketLength)
	binary.BigEndian.PutUint16(packet[4:6], header.HeaderLength)
	binary.BigEndian.PutUint16(packet[6:8], header.Version)
	binary.BigEndian.PutUint32(packet[8:12], header.Operation)
	binary.BigEndian.PutUint32(packet[12:16], header.SequenceID)

	return packet
}

// 处理心跳
func HandleHeartbeat(conn *websocket.Conn, cm *manager.ConnectionManager) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			heartbeat := CreateHeartbeatPacket()
			if err := conn.WriteMessage(websocket.BinaryMessage, heartbeat); err != nil {
				cm.IncrementErrors()
				return
			}
		}
	}
}
