package rtmp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"ken/server/amf"
)

// Chunk Message Header - "fmt" field values
const (
	// Chunks of Type 0 are 11 bytes long. This type MUST be used at the
	// start of a chunk stream, and whenever the stream timestamp goes
	// backward (e.g., because of a backward seek).
	//
	//  0                   1                   2                   3
	//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                   timestamp                   |message length |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |     message length (cont)     |message type id| msg stream id |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |           message stream id (cont)            |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	//       Figure 9 Chunk Message Header – Type 0
	HEADER_FMT_FULL = 0x00

	// Chunks of Type 1 are 7 bytes long. The message stream ID is not
	// included; this chunk takes the same stream ID as the preceding chunk.
	// Streams with variable-sized messages (for example, many video
	// formats) SHOULD use this format for the first chunk of each new
	// message after the first.
	//
	//  0                   1                   2                   3
	//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                timestamp delta                |message length |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |     message length (cont)     |message type id|
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	//       Figure 10 Chunk Message Header – Type 1
	HEADER_FMT_SAME_STREAM = 0x01

	// Chunks of Type 2 are 3 bytes long. Neither the stream ID nor the
	// message length is included; this chunk has the same stream ID and
	// message length as the preceding chunk. Streams with constant-sized
	// messages (for example, some audio and data formats) SHOULD use this
	// format for the first chunk of each message after the first.
	//
	//  0                   1                   2
	//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                timestamp delta                |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	//       Figure 11 Chunk Message Header – Type 2
	HEADER_FMT_SAME_LENGTH_AND_STREAM = 0x02

	// Chunks of Type 3 have no header. Stream ID, message length and
	// timestamp delta are not present; chunks of this type take values from
	// the preceding chunk. When a single message is split into chunks, all
	// chunks of a message except the first one, SHOULD use this type. Refer
	// to example 2 in section 6.2.2. Stream consisting of messages of
	// exactly the same size, stream ID and spacing in time SHOULD use this
	// type for all chunks after chunk of Type 2. Refer to example 1 in
	// section 6.2.1. If the delta between the first message and the second
	// message is same as the time stamp of first message, then chunk of
	// type 3 would immediately follow the chunk of type 0 as there is no
	// need for a chunk of type 2 to register the delta. If Type 3 chunk
	// follows a Type 0 chunk, then timestamp delta for this Type 3 chunk is
	// the same as the timestamp of Type 0 chunk.
	HEADER_FMT_CONTINUATION = 0x03
)

// Result codes
const (
	RESULT_CONNECT_OK            = "NetConnection.Connect.Success"
	RESULT_CONNECT_REJECTED      = "NetConnection.Connect.Rejected"
	RESULT_CONNECT_OK_DESC       = "Connection successed."
	RESULT_CONNECT_REJECTED_DESC = "[ AccessManager.Reject ] : [ code=400 ] : "
	NETSTREAM_PLAY_START         = "NetStream.Play.Start"
	NETSTREAM_PLAY_RESET         = "NetStream.Play.Reset"
	NETSTREAM_PUBLISH_START      = "NetStream.Publish.Start"
)

// Chunk stream ID
const (
	CS_ID_PROTOCOL_CONTROL = uint32(2)
	CS_ID_COMMAND          = uint32(3)
	CS_ID_USER_CONTROL     = uint32(4)
)

// Message type
const (
	// Set Chunk Size
	//
	// Protocol control message 1, Set Chunk Size, is used to notify the
	// peer a new maximum chunk size to use.

	// The value of the chunk size is carried as 4-byte message payload. A
	// default value exists for chunk size, but if the sender wants to
	// change this value it notifies the peer about it through this
	// protocol message. For example, a client wants to send 131 bytes of
	// data and the chunk size is at its default value of 128. So every
	// message from the client gets split into two chunks. The client can
	// choose to change the chunk size to 131 so that every message get
	// split into two chunks. The client MUST send this protocol message to
	// the server to notify that the chunk size is set to 131 bytes.
	// The maximum chunk size can be 65536 bytes. Chunk size is maintained
	// independently for server to client communication and client to server
	// communication.
	//
	//  0                   1                   2                   3
	//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                          chunk size (4 bytes)                 |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// Figure 2 Pay load for the protocol message ‘Set Chunk Size’
	//
	// chunk size: 32 bits
	//   This field holds the new chunk size, which will be used for all
	//   future chunks sent by this chunk stream.
	SET_CHUNK_SIZE = uint8(1)

	// Abort Message
	//
	// Protocol control message 2, Abort Message, is used to notify the peer
	// if it is waiting for chunks to complete a message, then to discard
	// the partially received message over a chunk stream and abort
	// processing of that message. The peer receives the chunk stream ID of
	// the message to be discarded as payload of this protocol message. This
	// message is sent when the sender has sent part of a message, but wants
	// to tell the receiver that the rest of the message will not be sent.
	//
	//  0                   1                   2                   3
	//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                        chunk stream id (4 bytes)              |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// Figure 3 Pay load for the protocol message ‘Abort Message’.
	//
	//
	// chunk stream ID: 32 bits
	//   This field holds the chunk stream ID, whose message is to be
	//   discarded.
	ABORT_MESSAGE = uint8(2)

	// Acknowledgement
	//
	// The client or the server sends the acknowledgment to the peer after
	// receiving bytes equal to the window size. The window size is the
	// maximum number of bytes that the sender sends without receiving
	// acknowledgment from the receiver. The server sends the window size to
	// the client after application connects. This message specifies the
	// sequence number, which is the number of the bytes received so far.
	//  0                   1                   2                   3
	//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                        sequence number (4 bytes)              |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// Figure 4 Pay load for the protocol message ‘Acknowledgement’.
	//
	// sequence number: 32 bits
	//   This field holds the number of bytes received so far.
	ACKNOWLEDGEMENT = uint8(3)

	// User Control Message
	//
	// The client or the server sends this message to notify the peer about
	// the user control events. This message carries Event type and Event
	// data.
	// +------------------------------+-------------------------
	// |     Event Type ( 2- bytes ) | Event Data
	// +------------------------------+-------------------------
	// Figure 5 Pay load for the ‘User Control Message’.
	//
	//
	// The first 2 bytes of the message data are used to identify the Event
	// type. Event type is followed by Event data. Size of Event data field
	// is variable.
	USER_CONTROL_MESSAGE = uint8(4)

	// Window Acknowledgement Size
	//
	// The client or the server sends this message to inform the peer which
	// window size to use when sending acknowledgment. For example, a server
	// expects acknowledgment from the client every time the server sends
	// bytes equivalent to the window size. The server updates the client
	// about its window size after successful processing of a connect
	// request from the client.
	//
	//  0                   1                   2                   3
	//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                   Acknowledgement Window size (4 bytes)       |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// Figure 6 Pay load for ‘Window Acknowledgement Size’.
	WINDOW_ACKNOWLEDGEMENT_SIZE = uint8(5)

	// Set Peer Bandwidth
	//
	// The client or the server sends this message to update the output
	// bandwidth of the peer. The output bandwidth value is the same as the
	// window size for the peer. The peer sends ‘Window Acknowledgement
	// Size’ back if its present window size is different from the one
	// received in the message.
	//  0                   1                   2                   3
	//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                   Acknowledgement Window size                 |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// | Limit type    |
	// +-+-+-+-+-+-+-+-+
	// Figure 7 Pay load for ‘Set Peer Bandwidth’
	//
	// The sender can mark this message hard (0), soft (1), or dynamic (2)
	// using the Limit type field. In a hard (0) request, the peer must send
	// the data in the provided bandwidth. In a soft (1) request, the
	// bandwidth is at the discretion of the peer and the sender can limit
	// the bandwidth. In a dynamic (2) request, the bandwidth can be hard or
	// soft.
	SET_PEER_BANDWIDTH = uint8(6)

	// Audio message
	//
	// The client or the server sends this message to send audio data to the
	// peer. The message type value of 8 is reserved for audio messages.
	AUDIO_TYPE = uint8(8)

	// Video message
	//
	// The client or the server sends this message to send video data to the
	// peer. The message type value of 9 is reserved for video messages.
	// These messages are large and can delay the sending of other type of
	// messages. To avoid such a situation, the video message is assigned
	// the lowest priority.
	VIDEO_TYPE = uint8(9)

	// Aggregate message
	//
	// An aggregate message is a single message that contains a list of sub-
	// messages. The message type value of 22 is reserved for aggregate
	// messages.
	AGGREGATE_MESSAGE_TYPE = uint8(22)

	// Shared object message
	//
	// A shared object is a Flash object (a collection of name value pairs)
	// that are in synchronization across multiple clients, instances, and
	// so on. The message types kMsgContainer=19 for AMF0 and
	// kMsgContainerEx=16 for AMF3 are reserved for shared object events.
	// Each message can contain multiple events.
	SHARED_OBJECT_AMF0 = uint8(19)
	SHARED_OBJECT_AMF3 = uint8(16)

	// Data message
	//
	// The client or the server sends this message to send Metadata or any
	// user data to the peer. Metadata includes details about the
	// data(audio, video etc.) like creation time, duration, theme and so
	// on. These messages have been assigned message type value of 18 for
	// AMF0 and message type value of 15 for AMF3.
	DATA_AMF0 = uint8(18)
	DATA_AMF3 = uint8(15)

	// Command message
	//
	// Command messages carry the AMF-encoded commands between the client
	// and the server. These messages have been assigned message type value
	// of 20 for AMF0 encoding and message type value of 17 for AMF3
	// encoding. These messages are sent to perform some operations like
	// connect, createStream, publish, play, pause on the peer. Command
	// messages like onstatus, result etc. are used to inform the sender
	// about the status of the requested commands. A command message
	// consists of command name, transaction ID, and command object that
	// contains related parameters. A client or a server can request Remote
	// Procedure Calls (RPC) over streams that are communicated using the
	// command messages to the peer.
	COMMAND_AMF0 = uint8(20)
	COMMAND_AMF3 = uint8(17) // Keng-die!!! Just ignore one byte before AMF0.
)

// User Control Message
//
// The client or the server sends this message to notify the peer about
// the user control events. This message carries Event type and Event
// data.
// +------------------------------+-------------------------
// |     Event Type ( 2- bytes )  | Event Data
// +------------------------------+-------------------------
// Figure 5 Pay load for the ‘User Control Message’.
//
//
// The first 2 bytes of the message data are used to identify the Event
// type. Event type is followed by Event data. Size of Event data field
// is variable.
//
//
// The client or the server sends this message to notify the peer about
// the user control events. For information about the message format,
// refer to the User Control Messages section in the RTMP Message
// Foramts draft.
//
// The following user control event types are supported:
// +---------------+--------------------------------------------------+
// |     Event     |                   Description                    |
// +---------------+--------------------------------------------------+
// |Stream Begin   | The server sends this event to notify the client |
// |        (=0)   | that a stream has become functional and can be   |
// |               | used for communication. By default, this event   |
// |               | is sent on ID 0 after the application connect    |
// |               | command is successfully received from the        |
// |               | client. The event data is 4-byte and represents  |
// |               | the stream ID of the stream that became          |
// |               | functional.                                      |
// +---------------+--------------------------------------------------+
// | Stream EOF    | The server sends this event to notify the client |
// |        (=1)   | that the playback of data is over as requested   |
// |               | on this stream. No more data is sent without     |
// |               | issuing additional commands. The client discards |
// |               | the messages received for the stream. The        |
// |               | 4 bytes of event data represent the ID of the    |
// |               | stream on which playback has ended.              |
// +---------------+--------------------------------------------------+
// | StreamDry     | The server sends this event to notify the client |
// |      (=2)     | that there is no more data on the stream. If the |
// |               | server does not detect any message for a time    |
// |               | period, it can notify the subscribed clients     |
// |               | that the stream is dry. The 4 bytes of event     |
// |               | data represent the stream ID of the dry stream.  |
// +---------------+--------------------------------------------------+
// | SetBuffer     | The client sends this event to inform the server |
// | Length (=3)   | of the buffer size (in milliseconds) that is     |
// |               | used to buffer any data coming over a stream.    |
// |               | This event is sent before the server starts      |
// |               | processing the stream. The first 4 bytes of the  |
// |               | event data represent the stream ID and the next  |
// |               | 4 bytes represent the buffer length, in          |
// |               | milliseconds.                                    |
// +---------------+--------------------------------------------------+
// | StreamIs      | The server sends this event to notify the client |
// | Recorded (=4) | that the stream is a recorded stream. The        |
// |               | 4 bytes event data represent the stream ID of    |
// |               | the recorded stream.                             |
// +---------------+--------------------------------------------------+
// | PingRequest   | The server sends this event to test whether the  |
// |       (=6)    | client is reachable. Event data is a 4-byte      |
// |               | timestamp, representing the local server time    |
// |               | when the server dispatched the command. The      |
// |               | client responds with kMsgPingResponse on         |
// |               | receiving kMsgPingRequest.                       |
// +---------------+--------------------------------------------------+
// | PingResponse  | The client sends this event to the server in     |
// |        (=7)   | response to the ping request. The event data is  |
// |               | a 4-byte timestamp, which was received with the  |
// |               | kMsgPingRequest request.                         |
// +---------------+--------------------------------------------------+
const (
	EVENT_STREAM_BEGIN       = uint16(0)
	EVENT_STREAM_EOF         = uint16(1)
	EVENT_STREAM_DRY         = uint16(2)
	EVENT_SET_BUFFER_LENGTH  = uint16(3)
	EVENT_STREAM_IS_RECORDED = uint16(4)
	EVENT_PING_REQUEST       = uint16(6)
	EVENT_PING_RESPONSE      = uint16(7)
	EVENT_REQUEST_VERIFY     = uint16(0x1a)
	EVENT_RESPOND_VERIFY     = uint16(0x1b)
	EVENT_BUFFER_EMPTY       = uint16(0x1f)
	EVENT_BUFFER_READY       = uint16(0x20)
)

const (
	BINDWIDTH_LIMIT_HARD    = uint8(0)
	BINDWIDTH_LIMIT_SOFT    = uint8(1)
	BINDWIDTH_LIMIT_DYNAMIC = uint8(2)
)

var (
	//	FLASH_PLAYER_VERSION = []byte{0x0A, 0x00, 0x2D, 0x02}
	FLASH_PLAYER_VERSION = []byte{0x09, 0x00, 0x7C, 0x02}
	// FLASH_PLAYER_VERSION = []byte{0x80, 0x00, 0x07, 0x02}
	// FLASH_PLAYER_VERSION_STRING = "LNX 10,0,32,18"
	FLASH_PLAYER_VERSION_STRING = "LNX 9,0,124,2"
	// FLASH_PLAYER_VERSION_STRING = "WIN 11,5,502,146"
	SWF_URL_STRING     = "http://localhost/1.swf"
	PAGE_URL_STRING    = "http://localhost/1.html"
	MIN_BUFFER_LENGTH  = uint32(256)
	FMS_VERSION        = []byte{0x04, 0x05, 0x00, 0x01}
	FMS_VERSION_STRING = "4,5,0,297"
)

const (
	MAX_TIMESTAMP                       = uint32(2000000000)
	AUTO_TIMESTAMP                      = uint32(0XFFFFFFFF)
	DEFAULT_HIGH_PRIORITY_BUFFER_SIZE   = 2048
	DEFAULT_MIDDLE_PRIORITY_BUFFER_SIZE = 128
	DEFAULT_LOW_PRIORITY_BUFFER_SIZE    = 64
	DEFAULT_CHUNK_SIZE                  = uint32(128)
	DEFAULT_WINDOW_SIZE                 = 2500000
	DEFAULT_CAPABILITIES                = float64(15)
	DEFAULT_AUDIO_CODECS                = float64(4071)
	DEFAULT_VIDEO_CODECS                = float64(252)
	FMS_CAPBILITIES                     = uint32(255)
	FMS_MODE                            = uint32(2)
	SET_PEER_BANDWIDTH_HARD             = byte(0)
	SET_PEER_BANDWIDTH_SOFT             = byte(1)
	SET_PEER_BANDWIDTH_DYNAMIC          = byte(2)
)

type Writer interface {
	Write(p []byte) (n int, err error)
	WriteByte(c byte) error
}

type Reader interface {
	Read(p []byte) (n int, err error)
	ReadByte() (c byte, err error)
}

type RtmpURL struct {
	protocol     string
	host         string
	port         uint16
	app          string
	instanceName string
}

func ParseURL(url string) (rURL RtmpURL, err error) {
	s1 := strings.SplitN(url, "://", 2)
	if len(s1) != 2 {
		err = fmt.Errorf("invalid url %s", url)
		return
	}
	rURL.protocol = strings.ToLower(s1[0])
	s1 = strings.SplitN(s1[1], "/", 2)
	if len(s1) != 2 {
		err = fmt.Errorf("invalid url %s", url)
		return
	}
	s2 := strings.SplitN(s1[0], ":", 2)
	if len(s2) == 2 {
		port, err := strconv.Atoi(s2[1])
		if err != nil || port > 65535 || port <= 0 {
			err = fmt.Errorf("invalid url %s", url)
			return
		}
		rURL.port = uint16(port)
	} else {
		rURL.port = 1935
	}
	if len(s2[0]) == 0 {
		err = fmt.Errorf("invalid url %s", url)
		return
	}
	rURL.host = s2[0]

	s2 = strings.SplitN(s1[1], "/", 2)
	rURL.app = s2[0]
	if len(s2) == 2 {
		rURL.instanceName = s2[1]
	}
	return
}

func (rURL *RtmpURL) App() string {
	if len(rURL.instanceName) == 0 {
		return rURL.app
	}
	return rURL.app + "/" + rURL.instanceName
}

func errPanic(err error, desc string) {
	if err != nil {
		panic(fmt.Errorf("%s: %s", desc, err.Error()))
	}
}

func writeString(w Writer, name, value string) (err error) {
	if _, err = amf.WriteObjectName(w, name); err != nil {
		return errors.WithMessage(err, "write object name")
	}
	if _, err = amf.WriteString(w, value); err != nil {
		return errors.WithMessage(err, "write value")
	}
	return
}

func writeDouble(w Writer, name string, value float64) (err error) {
	if _, err = amf.WriteObjectName(w, name); err != nil {
		return errors.WithMessage(err, "write object name")
	}
	if _, err = amf.WriteDouble(w, value); err != nil {
		return errors.WithMessage(err, "write value")
	}
	return
}

func GetTimestamp() uint32 {
	return uint32(time.Now().UnixNano()/int64(1000000)) % MAX_TIMESTAMP
}

func ReadByteFromNetwork(r Reader) (b byte, err error) {
	retry := 1
	for {
		if b, err = r.ReadByte(); err == nil {
			return
		}
		netErr, ok := err.(net.Error)
		if !ok || !netErr.Temporary() {
			return
		}
		logger.Debugf("Read byte from network blocked")
		if retry < 16 {
			retry = retry * 2
		}
		time.Sleep(time.Duration(retry*100) * time.Millisecond)
	}
}

func ReadAtLeastFromNetwork(r Reader, buf []byte, min int) (n int, err error) {
	retry := 1
	for {
		if n, err = io.ReadAtLeast(r, buf, min); err == nil {
			return
		}
		netErr, ok := err.(net.Error)
		if !ok || !netErr.Temporary() {
			return
		}
		if retry < 16 {
			retry = retry * 2
		}
		time.Sleep(time.Duration(retry*100) * time.Millisecond)
	}
}

func WriteToNetwork(w Writer, data []byte) (nn int, err error) {
	length := len(data)
	var n int
	retry := 1
	for nn < length {
		n, err = w.Write(data[nn:])
		if err == nil {
			nn += n
			continue
		}
		netErr, ok := err.(net.Error)
		if !ok || !netErr.Temporary() {
			return
		}
		if retry < 16 {
			retry = retry * 2
		}
		time.Sleep(time.Duration(retry*500) * time.Millisecond)
	}
	return
}

func CopyToNetwork(dst Writer, src Reader, n int64) (nn int64, err error) {
	buf := make([]byte, 4096)
	for nn < n {
		l := len(buf)
		if d := n - nn; d < int64(l) {
			l = int(d)
		}
		nr, er := io.ReadAtLeast(src, buf[0:l], l)
		if nr > 0 {
			nw, ew := WriteToNetwork(dst, buf[0:nr])
			if nw > 0 {
				nn += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			err = er
			break
		}
	}
	return
}

func FlushToNetwork(w *bufio.Writer) (err error) {
	retry := 1
	for {
		if err = w.Flush(); err == nil {
			return
		}
		netErr, ok := err.(net.Error)
		if !ok || !netErr.Temporary() {
			return
		}
		if retry < 16 {
			retry *= 2
		}
		time.Sleep(time.Duration(retry*500) * time.Millisecond)
	}
}
