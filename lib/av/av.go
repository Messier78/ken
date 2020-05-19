package av

import "errors"

const (
	AUDIO_TYPE = uint8(8)
	VIDEO_TYPE = uint8(9)
	META_TYPE  = uint8(18)
)

// ErrSendTooMuch means a player has received too many packets
var ErrSendTooMuch = errors.New("send too much")

// ErrNoPacketInCache means there is no packet that a player can read
var ErrNoPacketInCache = errors.New("no packet in cache")
