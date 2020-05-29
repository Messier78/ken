package rtmp

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"

	"ken/lib/amf"
	"ken/lib/av"
)

type Conn interface {
	Close()
	Send(msg *Message) error
	Flush() error
	CreateChunkStream(id uint32) (*OutboundChunkStream, error)
	CloseChunkStream(id uint32)
	NewTransactionID() uint32
	CreateMediaChunkStream() (*OutboundChunkStream, error)
	CloseMediaChunkStream(id uint32)
	SetStreamBufferSize(streamID uint32, size uint32)
	OutboundChunkStream(id uint32) (cs *OutboundChunkStream, found bool)
	InboundChunkStream(id uint32) (cs *InboundChunkStream, found bool)
	SetWindowAcknowledgementSize()
	SetPeerBandwidth(peerBandwidth uint32, limitType byte)
	SetChunkSize(chunkSize uint32)
	SendUserControlMessage(eventID uint16)
}

type ConnHandler interface {
	OnReceived(conn Conn, msg *Message)
	OnReceivedRtmpCommand(conn Conn, command *Command)
	OnClosed(conn Conn)
}

type conn struct {
	// streams
	outChunkStreams map[uint32]*OutboundChunkStream
	inChunkStreams  map[uint32]*InboundChunkStream
	keyString       string

	// Chunk size
	inChunkSize      uint32
	outChunkSize     uint32
	outChunkSizeTemp uint32

	// Bytes counter (for window ack)
	inBytes  uint32
	outBytes uint32

	// Previous window acknowledgement inbytes
	inBytesPreWindow uint32

	// Window size
	inWindowSize  uint32
	outWindowSize uint32

	// Bandwidth
	inBandwidth  uint32
	outBandwidth uint32

	// Bandwidth Limit
	inBandwidthLimit  uint8
	outBandwidthLimit uint8

	// Media chunk stream ID
	mediaChunkStreamIDAllocator       []bool
	mediaChunkStreamIDAllocatorLocker sync.Mutex

	closed  bool
	handler ConnHandler
	c       net.Conn
	br      *bufio.Reader
	bw      *bufio.Writer

	lastTransactionID uint32
	err               error
}

func NewConn(c net.Conn, br *bufio.Reader, bw *bufio.Writer,
	handler ConnHandler, maxChannelNumber int) Conn {
	conn := &conn{
		outChunkStreams:             make(map[uint32]*OutboundChunkStream),
		inChunkStreams:              make(map[uint32]*InboundChunkStream),
		inChunkSize:                 DEFAULT_CHUNK_SIZE,
		outChunkSize:                DEFAULT_CHUNK_SIZE,
		outChunkSizeTemp:            0,
		inBytes:                     0,
		outBytes:                    0,
		inBytesPreWindow:            0,
		inWindowSize:                DEFAULT_WINDOW_SIZE,
		outWindowSize:               DEFAULT_WINDOW_SIZE,
		inBandwidth:                 DEFAULT_WINDOW_SIZE,
		outBandwidth:                DEFAULT_WINDOW_SIZE,
		inBandwidthLimit:            BINDWIDTH_LIMIT_DYNAMIC,
		outBandwidthLimit:           BINDWIDTH_LIMIT_DYNAMIC,
		mediaChunkStreamIDAllocator: make([]bool, maxChannelNumber),
		closed:                      false,
		handler:                     handler,
		c:                           c,
		br:                          br,
		bw:                          bw,
		lastTransactionID:           0,
		err:                         nil,
	}

	// Create Protocal control chunk stream
	conn.outChunkStreams[CS_ID_PROTOCOL_CONTROL] = NewOutboundChunkStream(CS_ID_PROTOCOL_CONTROL)
	// Create Command message chunk stream
	conn.outChunkStreams[CS_ID_COMMAND] = NewOutboundChunkStream(CS_ID_COMMAND)
	// Create User control chunk stream
	conn.outChunkStreams[CS_ID_USER_CONTROL] = NewOutboundChunkStream(CS_ID_USER_CONTROL)
	// go conn.sendLoop()
	go conn.recvLoop()

	return conn
}

func (conn *conn) Close() {
	conn.closed = true
	conn.c.Close()
}

func (conn *conn) Send(msg *Message) error {
	err := conn.sendMessage(msg)
	if err != nil {
		logger.Errorf("Send message err: %s", err.Error())
	}
	return err
}

func (conn *conn) Flush() error {
	return conn.bw.Flush()
}

func (conn *conn) CreateChunkStream(id uint32) (*OutboundChunkStream, error) {
	cs, ok := conn.outChunkStreams[id]
	if ok {
		return nil, errors.New("ChunkStream existed")
	}
	cs = NewOutboundChunkStream(id)
	conn.outChunkStreams[id] = cs
	return cs, nil
}

func (conn *conn) CloseChunkStream(id uint32) {
	delete(conn.outChunkStreams, id)
}

func (conn *conn) NewTransactionID() uint32 {
	return atomic.AddUint32(&conn.lastTransactionID, 1)
}

func (conn *conn) CreateMediaChunkStream() (cs *OutboundChunkStream, err error) {
	var csi uint32
	conn.mediaChunkStreamIDAllocatorLocker.Lock()
	for idx, v := range conn.mediaChunkStreamIDAllocator {
		if !v {
			csi = uint32((idx+1)*6 + 2)
			conn.mediaChunkStreamIDAllocator[idx] = true
			break
		}
	}
	conn.mediaChunkStreamIDAllocatorLocker.Unlock()
	if csi == 0 {
		return nil, errors.New("No more chunk stream ID to allocate")
	}
	if cs, err = conn.CreateChunkStream(csi); err != nil {
		conn.CloseMediaChunkStream(csi)
		return nil, err
	}
	return
}

func (conn *conn) CloseMediaChunkStream(id uint32) {
	idx := (id-2)/6 - 1
	conn.mediaChunkStreamIDAllocatorLocker.Lock()
	conn.mediaChunkStreamIDAllocator[idx] = false
	conn.mediaChunkStreamIDAllocatorLocker.Unlock()
	conn.CloseChunkStream(id)
}

func (conn *conn) SetStreamBufferSize(streamID uint32, size uint32) {
	logger.Debugf("SetStreamBufferSize, id: %d, size: %d", streamID, size)
	msg := NewMessage(CS_ID_PROTOCOL_CONTROL, USER_CONTROL_MESSAGE, 0, 1, nil)
	eventType := EVENT_SET_BUFFER_LENGTH
	var err error
	if err = binary.Write(msg.Buf, binary.BigEndian, &eventType); err != nil {
		logger.Errorf("SetStreamBufferSize write event type EVENT_SET_BUFFER_LENGTH err: %s", err.Error())
		return
	}
	if err = binary.Write(msg.Buf, binary.BigEndian, &streamID); err != nil {
		logger.Errorf("SetStreamBufferSize write stream id err: %s", err.Error())
		return
	}
	if err = binary.Write(msg.Buf, binary.BigEndian, &size); err != nil {
		logger.Errorf("SetStreamBufferSize write size err: %s", err.Error())
		return
	}
	conn.Send(msg)
}

func (conn *conn) OutboundChunkStream(id uint32) (cs *OutboundChunkStream, found bool) {
	cs, found = conn.outChunkStreams[id]
	return
}

func (conn *conn) InboundChunkStream(id uint32) (cs *InboundChunkStream, found bool) {
	cs, found = conn.inChunkStreams[id]
	return
}

func (conn *conn) SetWindowAcknowledgementSize() {
	logger.Debugf("SetWindowAcknowledgementSize")
	msg := NewMessage(CS_ID_PROTOCOL_CONTROL, WINDOW_ACKNOWLEDGEMENT_SIZE, 0, 0, nil)
	if err := binary.Write(msg.Buf, binary.BigEndian, &conn.outWindowSize); err != nil {
		logger.Errorf("SetWindowAcknowledgementSize wrtie window size %d err: %d", conn.outWindowSize, err.Error())
		return
	}
	msg.Size = uint32(msg.Buf.Len())
	conn.Send(msg)
}

func (conn *conn) SetPeerBandwidth(peerBandwidth uint32, limitType byte) {
	logger.Debugf("setPeerBandwidth: %d - %c", peerBandwidth, limitType)
	msg := NewMessage(CS_ID_PROTOCOL_CONTROL, SET_PEER_BANDWIDTH, 0, 0, nil)
	if err := binary.Write(msg.Buf, binary.BigEndian, &peerBandwidth); err != nil {
		logger.Errorf("setPeerBandwidth write peerBandwidth err: %s", err.Error())
		return
	}
	if err := msg.Buf.WriteByte(limitType); err != nil {
		logger.Errorf("setPeerBandwidth write limitType err: %s", err.Error())
		return
	}
	msg.Size = uint32(msg.Buf.Len())
	conn.Send(msg)
}

func (conn *conn) SetChunkSize(size uint32) {
	logger.Debugf("Set out chunk size, size: %d", size)
	msg := NewMessage(CS_ID_PROTOCOL_CONTROL, SET_CHUNK_SIZE, 0, 0, nil)
	if err := binary.Write(msg.Buf, binary.BigEndian, &size); err != nil {
		logger.Errorf("SetChunkSize write size err: %s", err.Error())
		return
	}
	conn.outChunkSizeTemp = size
	conn.Send(msg)
}

func (conn *conn) SendUserControlMessage(eventID uint16) {
	logger.Debugf("sendUserControlMessage: event id: %d", eventID)
	msg := NewMessage(CS_ID_PROTOCOL_CONTROL, USER_CONTROL_MESSAGE, 0, 0, nil)
	if err := binary.Write(msg.Buf, binary.BigEndian, &eventID); err != nil {
		logger.Errorf("sendUserControlMessage write event type USER_CONTROL_MESSAGE err: %s", err.Error())
		return
	}
	var temp uint32
	temp = 0
	if err := binary.Write(msg.Buf, binary.BigEndian, &temp); err != nil {
	}
	conn.Send(msg)
}

/*
func (conn *conn) sendLoop() {
	defer func() {
		if r := recover(); r != nil {
			if conn.err == nil {
				conn.err = r.(error)
			}
		}
		conn.Close()
	}()
	for {
		select {
		case <-conn.ctx.Done():
			return
		case msg := <-conn.highPriorityMessageQueue:
			conn.sendMessage(msg)
		case msg := <-conn.middlePriorityMessageQueue:
			conn.sendMessage(msg)
			conn.checkAndSendHighPriorityMessage()
		case msg := <-conn.lowPriorityMessageQueue:
			conn.checkAndSendHighPriorityMessage()
			conn.sendMessage(msg)
		case <-time.After(time.Second):
		}
	}
}
*/

func (conn *conn) recvLoop() {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("panic error: %s", r.(error).Error())
			if conn.err == nil {
				conn.err = r.(error)
			}
		}
		conn.Close()
		conn.handler.OnClosed(conn)
	}()

	var cs *InboundChunkStream
	var ok bool
	var remain uint32
	for !conn.closed {
		n, vfmt, csi, err := ReadBaseHeader(conn.br)
		errPanic(err, "Read header 1")
		conn.inBytes += uint32(n)
		if cs, ok = conn.inChunkStreams[csi]; !ok || cs == nil {
			cs = NewInboundChunkStream(csi)
			conn.inChunkStreams[csi] = cs
		}

		// Read header
		header := &Header{}
		n, err = header.ReadHeader(conn.br, vfmt, csi, cs.lastHeader)
		errPanic(err, "Read Header 2")
		conn.inBytes += uint32(n)

		var absoluteTimestamp uint32
		var msg *Message
		switch vfmt {
		case HEADER_FMT_FULL:
			cs.lastHeader = header
			absoluteTimestamp = header.Timestamp
		case HEADER_FMT_SAME_STREAM:
			if cs.lastHeader == nil {
				logger.Debugf("New message with fmt: %d, csi: %d", vfmt, csi)
			} else {
				header.MessageStreamID = cs.lastHeader.MessageStreamID
			}
			cs.lastHeader = header
			absoluteTimestamp = cs.lastInAbsoluteTimestamp + header.Timestamp
		case HEADER_FMT_SAME_LENGTH_AND_STREAM:
			if cs.lastHeader == nil {
				logger.Warnf("New Message with fmt: %d, csi: %d", vfmt, csi)
			} else {
				header.MessageStreamID = cs.lastHeader.MessageStreamID
				header.MessageLength = cs.lastHeader.MessageLength
				header.MessageTypeID = cs.lastHeader.MessageTypeID
			}
			cs.lastHeader = header
			absoluteTimestamp = cs.lastInAbsoluteTimestamp + header.Timestamp
		case HEADER_FMT_CONTINUATION:
			if cs.receivedMessage != nil {
				msg = cs.receivedMessage
			}
			if cs.lastHeader == nil {
				logger.Warnf("New Message with fmt: %d, csi: %d", vfmt, csi)
			} else {
				header.MessageStreamID = cs.lastHeader.MessageStreamID
				header.MessageLength = cs.lastHeader.MessageLength
				header.MessageTypeID = cs.lastHeader.MessageTypeID
				header.Timestamp = cs.lastHeader.Timestamp
			}
			cs.lastHeader = header
			absoluteTimestamp = cs.lastInAbsoluteTimestamp
		}
		// logger.Debugf("absolute timestamp: %d", absoluteTimestamp)

		if msg == nil {
			msg = &Message{
				ChunkStreamID:     csi,
				Type:              header.MessageTypeID,
				Timestamp:         header.RealTimestamp(),
				Size:              header.MessageLength,
				StreamID:          header.MessageStreamID,
				Buf:               av.AcquirePacket(),
				IsInbound:         true,
				AbsoluteTimestamp: absoluteTimestamp,
			}
		}
		cs.lastInAbsoluteTimestamp = absoluteTimestamp
		// Read data
		remain = msg.Remain()
		var n64 int64
		if remain <= conn.inChunkSize {
			// One chunk message
			for {
				n64, err = io.CopyN(msg.Buf, conn.br, int64(remain))
				if err == nil {
					conn.inBytes += uint32(n64)
					if remain <= uint32(n64) {
						break
					} else {
						remain -= uint32(n64)
						continue
					}
				}
				netErr, ok := err.(net.Error)
				if !ok || !netErr.Temporary() {
					errPanic(err, "Read data 1")
				}
				logger.Errorf("%+v", errors.Wrap(err, "message copy blocked"))
			}
			conn.received(msg)
			cs.receivedMessage = nil
		} else {
			remain = conn.inChunkSize
			for {
				n64, err = io.CopyN(msg.Buf, conn.br, int64(remain))
				if err == nil {
					conn.inBytes += uint32(n64)
					if remain <= uint32(n64) {
						break
					} else {
						remain -= uint32(n64)
						continue
					}
				}
				netErr, ok := err.(net.Error)
				if !ok || !netErr.Temporary() {
					errPanic(err, "Read data 2")
				}
				logger.Errorf("%+v", errors.Wrap(err, "message copy blocked"))
			}
			cs.receivedMessage = msg
		}

		if conn.inBytes > (conn.inBytesPreWindow + conn.inWindowSize) {
			ackMsg := NewMessage(CS_ID_PROTOCOL_CONTROL, ACKNOWLEDGEMENT, 0, absoluteTimestamp+1, nil)
			errPanic(binary.Write(ackMsg.Buf, binary.BigEndian, conn.inBytes), "ACK Message write data")
			conn.inBytesPreWindow = conn.inBytes
			conn.Send(ackMsg)
		}
	}
}

func (conn *conn) error(err error, desc string) {
	// logger.Errorf("%+v", errors.Wrap(err, desc))
	if conn.err != nil {
		conn.err = err
	}
	conn.Close()
}

func (conn *conn) received(msg *Message) {
	tmpBuf := make([]byte, 4)
	var err error
	var subType, timestampExt byte
	var dataSize, timestamp uint32
	if msg.Type == AGGREGATE_MESSAGE_TYPE {
		var firstAggregateTimestamp uint32
		for msg.Buf.Len() > 0 {
			// Byte stream order
			// Sub message type 1 byte
			if subType, err = msg.Buf.ReadByte(); err != nil {
				logger.Errorf("AGGREGATE_MESSAGE_TYPE read sub type err: %s", err.Error())
				return
			}

			// Data size 3 bytes, big endian
			if _, err = io.ReadAtLeast(msg.Buf, tmpBuf[1:], 3); err != nil {
				logger.Errorf("AGGREGATE_MESSAGE_TYPE read data size err: %s", err.Error())
				return
			}
			dataSize = binary.BigEndian.Uint32(tmpBuf)

			// Timestamp 3 bytes
			if _, err = io.ReadAtLeast(msg.Buf, tmpBuf[1:], 3); err != nil {
				logger.Errorf("AGGREGATE_MESSAGE_TYPE read timestamp err: %s", err.Error())
				return
			}
			timestamp = binary.BigEndian.Uint32(tmpBuf)

			// Timestamp extend 1 byte,  result = (result >>> 8) | ((result & 0x000000ff) << 24);
			if timestampExt, err = msg.Buf.ReadByte(); err != nil {
				logger.Errorf("AGGREGATE_MESSAGE_TYPE read timestamp extend err: %s", err.Error())
				return
			}
			timestamp |= uint32(timestampExt) << 24
			if firstAggregateTimestamp == 0 {
				firstAggregateTimestamp = timestamp
			}

			// 3 bytes ignored
			if _, err = io.ReadAtLeast(msg.Buf, tmpBuf[1:], 3); err != nil {
				logger.Errorf("AGGREGATE_MESSAGE_TYPE read ignore bytes err: %s", err.Error())
				return
			}

			// Data
			subMsg := NewMessage(msg.ChunkStreamID, subType, msg.StreamID, 0, nil)
			subMsg.Timestamp = 0
			subMsg.IsInbound = true
			subMsg.Size = dataSize
			subMsg.AbsoluteTimestamp = msg.AbsoluteTimestamp
			if _, err = io.CopyN(subMsg.Buf, msg.Buf, int64(dataSize)); err != nil {
				logger.Errorf("AGGREGATE_MESSAGE_TYPE copy data err: %s", err.Error())
				return
			}
			conn.received(subMsg)

			// Previous tag size 4 bytes
			if msg.Buf.Len() >= 4 {
				if _, err = io.ReadAtLeast(msg.Buf, tmpBuf, 4); err != nil {
					logger.Errorf("AGGREGATE_MESSAGE_TYPE read previous tag size err: %s", err.Error())
					return
				}
			} else {
				logger.Errorf("AGGREGATE_MESSAGE_TYPE miss previous tag size")
				break
			}
		}
	} else {
		switch msg.ChunkStreamID {
		case CS_ID_PROTOCOL_CONTROL:
			switch msg.Type {
			case SET_CHUNK_SIZE:
				conn.invokeSetChunkSize(msg)
			case ABORT_MESSAGE:
				conn.invokeAbortMessage(msg)
			case ACKNOWLEDGEMENT:
				conn.invokeAcknowledgement(msg)
			case USER_CONTROL_MESSAGE:
				conn.invokeUserControlMessage(msg)
			case WINDOW_ACKNOWLEDGEMENT_SIZE:
				conn.invokeWindowAcknowledgementSize(msg)
			case SET_PEER_BANDWIDTH:
				conn.invokeSetPeerBandwidth(msg)
			default:
				logger.Debugf("unknown message type %d in protocal control chunk stream", msg.Type)
			}
		case CS_ID_COMMAND:
			if err = conn.receivedCommand(msg); err != nil {
				return
			}
		default:
			conn.handler.OnReceived(conn, msg)
		}
	}
}

func (conn *conn) receivedCommand(msg *Message) (err error) {
	if msg.StreamID == 0 {
		cmd := &Command{}
		var transactionID float64
		var object interface{}
		switch msg.Type {
		case COMMAND_AMF3:
			cmd.IsFlex = true
			if _, err = msg.Buf.ReadByte(); err != nil {
				logger.Errorf("read first in flex command err: %s", err.Error())
				return
			}
			fallthrough
		case COMMAND_AMF0:
			if cmd.Name, err = amf.ReadString(msg.Buf); err != nil {
				logger.Errorf("AMF0 read name err: %s", err.Error())
				return
			}
			if transactionID, err = amf.ReadDouble(msg.Buf); err != nil {
				logger.Errorf("AMF0 read transcationId err: %s", err.Error())
				return
			}
			cmd.TransactionID = uint32(transactionID)
			for msg.Buf.Len() > 0 {
				if object, err = amf.ReadValue(msg.Buf); err != nil {
					logger.Errorf("AMF0 read object err: %s", err.Error())
					return
				}
				cmd.Objects = append(cmd.Objects, object)
			}
		default:
			logger.Debugf("Unknown message type %d in command chunk stream", msg.Type)
		}
		conn.invokeCommand(cmd)
	} else {
		conn.handler.OnReceived(conn, msg)
	}
	return
}

func (conn *conn) sendMessage(msg *Message) (err error) {
	var n int
	cs, ok := conn.outChunkStreams[msg.ChunkStreamID]
	if !ok || cs == nil {
		return
	}

	header := cs.NewOutboundHeader(msg)
	// header := cs.NewFixedOutboundHeader(msg)
	// logger.Debugf(">>> header.timestamp: %d, delta: %d, chunksize: %d", header.Timestamp, msg.Timestamp, conn.outChunkSize)
	if _, err = header.Write(conn.bw); err != nil {
		conn.error(err, "send message write header")
		return
	}

	if header.MessageLength > conn.outChunkSize {
		// split into chunks
		if n, err = conn.bw.Write(msg.Buf.Next(int(conn.outChunkSize))); err != nil || n != int(conn.outChunkSize) {
			conn.error(err, "send message copy buffer")
			return
		}

		remain := header.MessageLength - conn.outChunkSize
		for {
			if err = conn.bw.WriteByte(0xc0 | byte(header.ChunkStreamID)); err != nil {
				conn.error(err, "send message write Type 3 chunk header")
				return
			}
			if remain > conn.outChunkSize {
				// if _, err = CopyToNetwork(conn.bw, msg.Buf, int64(conn.outChunkSize)); err != nil {
				if n, err = conn.bw.Write(msg.Buf.Next(int(conn.outChunkSize))); err != nil || n != int(conn.outChunkSize) {
					conn.error(err, "send message copy split buffer 1")
					return
				}
				remain -= conn.outChunkSize
			} else {
				if n, err = conn.bw.Write(msg.Buf.Next(int(remain))); err != nil || n != int(remain) {
					conn.error(err, "send message copy split buffer 2")
					return
				}
				break
			}
		}
	} else {
		if n, err = conn.bw.Write(msg.Buf.Next(int(header.MessageLength))); err != nil || n != int(header.MessageLength) {
			conn.error(err, "send message copy buffer")
			return
		}
	}
	// Flush() should not be called every time a packet was sent
	// if err = conn.bw.Flush(); err != nil {
	// 	conn.error(err, "send message, fulsh 3")
	// 	return
	// }
	if msg.ChunkStreamID == CS_ID_PROTOCOL_CONTROL && msg.Type == SET_CHUNK_SIZE && conn.outChunkSizeTemp != 0 {
		logger.Infof("set out chunk size: %d", conn.outChunkSizeTemp)
		conn.outChunkSize = conn.outChunkSizeTemp
		conn.outChunkSizeTemp = 0
	}
	// TODO: recycle msg
	return
}

func (conn *conn) invokeSetChunkSize(msg *Message) {
	if err := binary.Read(msg.Buf, binary.BigEndian, &conn.inChunkSize); err != nil {
		logger.Warnf("conn::invokeSetChunkSize err: %s", err.Error())
	}
	logger.Debugf("set chunk size: %d", conn.inChunkSize)
}

func (conn *conn) invokeAbortMessage(msg *Message) {
	logger.Debugf("conn::invokeAbortMessage")
}

func (conn *conn) invokeAcknowledgement(msg *Message) {
	// logger.Debugf("conn::invokeAcknowledgement(): % 2x", msg.Buf.Bytes())
}

func (conn *conn) invokeUserControlMessage(msg *Message) {
	var eventType uint16
	var err error
	if err = binary.Read(msg.Buf, binary.BigEndian, &eventType); err != nil {
		logger.Errorf("read event type err: %s", err.Error())
	}
	switch eventType {
	case EVENT_STREAM_BEGIN:
		logger.Debugf("userControlMessage: EVENT_STREAM_BEGIN")
	case EVENT_STREAM_EOF:
		logger.Debugf("userControlMessage: EVENT_STREAM_EOF")
	case EVENT_STREAM_DRY:
		logger.Debugf("userControlMessage: EVENT_STREAM_DRY")
	case EVENT_SET_BUFFER_LENGTH:
		logger.Debugf("userControlMessage: EVENT_SET_BUFFER_LENGTH")
	case EVENT_STREAM_IS_RECORDED:
		logger.Debugf("userControlMessage: EVENT_STREAM_IS_RECORDED")
	case EVENT_PING_REQUEST:
		logger.Debugf("userControlMessage: EVENT_PING_REQUEST")
		var serverTimestamp uint32
		if err = binary.Read(msg.Buf, binary.BigEndian, &serverTimestamp); err != nil {
			logger.Errorf("read server timestamp err: %s", err.Error())
			return
		}
		respMsg := NewMessage(CS_ID_PROTOCOL_CONTROL, USER_CONTROL_MESSAGE, 0, msg.Timestamp+1, nil)
		respEventType := uint16(EVENT_PING_RESPONSE)
		if err = binary.Write(respMsg.Buf, binary.BigEndian, &respEventType); err != nil {
			logger.Errorf("write event type EVENT_PING RESPONSE err: %s", err.Error())
			return
		}
		if err = binary.Write(respMsg.Buf, binary.BigEndian, &serverTimestamp); err != nil {
			logger.Errorf("write EVENT_PING_RESPONSE server timestamp err: %s", err.Error())
			return
		}
		conn.Send(respMsg)
	case EVENT_PING_RESPONSE:
		logger.Debugf("userControlMessage: EVENT_PING_RESPONSE")
	case EVENT_REQUEST_VERIFY:
		logger.Debugf("userControlMessage: EVENT_REQUEST_VERIFY")
	case EVENT_RESPOND_VERIFY:
		logger.Debugf("userControlMessage: EVENT_RESPOND_VERIFY")
	case EVENT_BUFFER_EMPTY:
		logger.Debugf("userControlMessage: EVENT_BUFFER_EMPTY")
	case EVENT_BUFFER_READY:
		logger.Debugf("userControlMessage: EVENT_BUFFER_READY")
	default:
		logger.Debugf("userControlMessage: Unknown user control message: 0x%x", eventType)
	}
}

func (conn *conn) invokeWindowAcknowledgementSize(msg *Message) {
	var size uint32
	if err := binary.Read(msg.Buf, binary.BigEndian, &size); err != nil {
		logger.Warnf("conn::invokeWindowAcknowledgementSize read window size err: %s", err.Error())
		return
	}
	conn.inWindowSize = size
	logger.Debugf("conn::invokeWindowAcknowledgementSize() set inWindowSize: %d", conn.inWindowSize)
}

func (conn *conn) invokeSetPeerBandwidth(msg *Message) {
	var err error
	var size uint32
	var limit byte
	if err = binary.Read(msg.Buf, binary.BigEndian, &size); err != nil {
		logger.Warnf("conn::invokeSetPeerBandwidth read window size err: %s", err.Error())
		return
	}
	conn.inBandwidth = size

	if limit, err = msg.Buf.ReadByte(); err != nil {
		logger.Warnf("conn::invokeSetPeerBandwidth read limit err: %s", err.Error())
		return
	}
	conn.inBandwidthLimit = uint8(limit)
	logger.Debugf("conn.inBandwidthLimit = %d", conn.inBandwidthLimit)
}

func (conn *conn) invokeCommand(cmd *Command) {
	logger.Debugf("conn::invokeCommand() ==> %s", cmd.Name)
	conn.handler.OnReceivedRtmpCommand(conn, cmd)
}
