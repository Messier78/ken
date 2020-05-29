package rtmp

import "ken/lib/av"

func CodecPacket(f *av.Packet) (ff *av.Packet) {
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
			// logger.Debugf(">>> key frame received")
			f.IsKeyFrame = true
		}
	}

	// codec
	fmt := buf[0]
	acodecID := (fmt & 0xf0) >> 4
	vcodecID := fmt & 0x0f

	if f.Type == AUDIO_TYPE && acodecID == 10 && buf[1] == 0 {
		f.IsCodec = true
		f.IsAAC = true
		logger.Debugf(">>> AAC")
	}
	if f.Type == VIDEO_TYPE && (vcodecID == 7 || vcodecID == 12) && buf[1] == 0 {
		f.IsCodec = true
		logger.Debugf(">>> AVC")
	}
	return
}
