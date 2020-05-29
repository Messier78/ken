// config
package av

import "time"

type Config struct {
	// millisecond
	AudioOnlyGopDuration uint32

	// DelayTime
	DelayTime uint32

	// LowLatency
	// if true, reader will reset node to the latest key frame
	LowLatency bool

	// maximum duration that a player can cache
	// player will sleep 500ms if so many frames were sent
	ClientDuration time.Duration

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

	// set cache.maxDelta to the max packet.Delta which is
	// Less than MaxAvaliableDelta
	MaxAvaliableDelta uint32

	// if packet.Delta is larger than MaxDelta, set it
	// to cache.maxDelta
	MaxDelta uint32
}

func initConfig() *Config {
	return &Config{
		AudioOnlyGopDuration: 5000,
		ClientDuration:       6000,
		DelayTime:            3000,
		RingSize:             1024,
		SessionTimeout:       60,
		Sync:                 300,
		MaxAvaliableDelta:    50,
		MaxDelta:             200,
	}
}
