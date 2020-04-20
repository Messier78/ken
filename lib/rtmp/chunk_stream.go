package rtmp

type OutboundChunkStream struct {
	ID uint32

	lastHeader               *Header
	lastOutAbsoluteTimestamp uint32
	lastInAbsoluteTimestamp  uint32

	startAt uint32
}

type InboundChunkStream struct {
	ID uint32

	lastHeader               *Header
	lastOutAbsoluteTimestamp uint32
	lastInAbsoluteTimestamp  uint32

	receivedMessage *Message
}

func NewOutboundChunkStream(id uint32) *OutboundChunkStream {
	return &OutboundChunkStream{
		ID: id,
	}
}

func NewInboundChunkStream(id uint32) *InboundChunkStream {
	return &InboundChunkStream{
		ID: id,
	}
}

func (cs *OutboundChunkStream) NewOutboundHeader(msg *Message) *Header {
	header := &Header{
		ChunkStreamID:   cs.ID,
		MessageLength:   uint32(msg.Buf.Len()),
		MessageTypeID:   msg.Type,
		MessageStreamID: msg.StreamID,
	}
	timestamp := msg.Timestamp
	if timestamp == AUTO_TIMESTAMP {
		timestamp = cs.GetTimestamp()
		msg.Timestamp = timestamp
		msg.AbsoluteTimestamp = timestamp
	}
	deltaTimestamp := uint32(0)
	if cs.lastOutAbsoluteTimestamp < msg.Timestamp {
		deltaTimestamp = msg.Timestamp - cs.lastOutAbsoluteTimestamp
	}
	if cs.lastHeader == nil {
		header.Fmt = HEADER_FMT_FULL
		header.Timestamp = timestamp
	} else {
		if header.MessageStreamID == cs.lastHeader.MessageStreamID {
			if header.MessageTypeID == cs.lastHeader.MessageTypeID && header.MessageLength == cs.lastHeader.MessageLength {
				switch cs.lastHeader.Fmt {
				case HEADER_FMT_FULL:
					header.Fmt = HEADER_FMT_SAME_LENGTH_AND_STREAM
					header.Timestamp = deltaTimestamp
				case HEADER_FMT_SAME_STREAM:
					fallthrough
				case HEADER_FMT_SAME_LENGTH_AND_STREAM:
					fallthrough
				case HEADER_FMT_CONTINUATION:
					if cs.lastHeader.Timestamp == deltaTimestamp {
						header.Fmt = HEADER_FMT_CONTINUATION
					} else {
						header.Fmt = HEADER_FMT_SAME_LENGTH_AND_STREAM
						header.Timestamp = deltaTimestamp
					}
				}
			} else {
				header.Fmt = HEADER_FMT_SAME_STREAM
				header.Timestamp = deltaTimestamp
			}
		} else {
			header.Fmt = HEADER_FMT_FULL
			header.Timestamp = timestamp
		}
	}
	// Check extended timestamp
	if header.Timestamp >= 0xffffff {
		header.ExtendTimeStamp = msg.Timestamp
		header.Timestamp = 0xffffff
	} else {
		header.ExtendTimeStamp = 0
	}

	logger.Debugf("OutboundChunkStream::NewOutboundHeader, header: %+v", header)
	cs.lastHeader = header
	cs.lastOutAbsoluteTimestamp = timestamp

	return header
}

func (cs *OutboundChunkStream) GetTimestamp() uint32 {
	if cs.startAt == uint32(0) {
		cs.startAt = GetTimestamp()
		return uint32(0)
	}
	return GetTimestamp() - cs.startAt
}
