package av

const (
	AUDIO_TYPE = uint8(8)
	VIDEO_TYPE = uint8(9)
	META_TYPE  = uint8(18)
)

type Reader interface {
	ReadPacket() (f *Packet, err error)
}

type Writer interface {
	WritePacket(f *Packet) error
}
