// config
package av

type Config struct {
	// millisecond
	AudioOnlyGopDuration uint32
	// DelayTime
	DelayTime uint32
	// RingSize
	RingSize int
	// SessionTimeout (s)
	// Cache will wait for a while even if no packet is received
	SessionTimeout uint32
	// Timestamp deviation threshold (ms)
	// if delta timestamp of packet is over Sync, it will be set
	// to the max delta timestamp received before
	Sync uint32
	// if false, players will be disconnected immidiately when no
	// cache writers avaliable
	IdleStreams bool
	// cache writer will wait for a while even if no packet is received
	DropIdleWriter uint32
}
