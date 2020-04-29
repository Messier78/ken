package rtmp

import "ken/lib/av"

func CodecPacket(s *inboundStream, f *av.Packet) (ff *av.Packet) {
	defer func() {
		ff = f
	}()
	if f.Len() < 2 {
		return
	}
	buf := f.Bytes()
	// keyframe
	if f.Type == VIDEO_TYPE {
		if buf[0]&0xf0 == 0x10 {
			f.IsKeyFrame = true
		}
	}

	// codec
	fmt := uint8(buf[0])
	acodecID := (fmt & 0xf0) >> 4
	vcodecID := fmt & 0x0f

	if f.Type == AUDIO_TYPE && acodecID == 10 ||
		(f.Type == VIDEO_TYPE && (vcodecID == 7 || vcodecID == 12)) {
		if buf[1] == 0 {
			f.IsCodec = true
		}
	}
	return
}
