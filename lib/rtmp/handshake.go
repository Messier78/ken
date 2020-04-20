package rtmp

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"time"
)

const (
	SIG_SIZE             = 1536
	LARGE_HEADER_SIZE    = 12
	SHA256_DIGEST_LENGTH = 32
	DEFAULT_CHUNKSIZE    = 128
)

var (
	GENUINE_FMS_KEY = []byte{
		0x47, 0x65, 0x6e, 0x75, 0x69, 0x6e, 0x65, 0x20,
		0x41, 0x64, 0x6f, 0x62, 0x65, 0x20, 0x46, 0x6c,
		0x61, 0x73, 0x68, 0x20, 0x4d, 0x65, 0x64, 0x69,
		0x61, 0x20, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72,
		0x20, 0x30, 0x30, 0x31, // Genuine Adobe Flash Media Server 001
		0xf0, 0xee, 0xc2, 0x4a, 0x80, 0x68, 0xbe, 0xe8,
		0x2e, 0x00, 0xd0, 0xd1, 0x02, 0x9e, 0x7e, 0x57,
		0x6e, 0xec, 0x5d, 0x2d, 0x29, 0x80, 0x6f, 0xab,
		0x93, 0xb8, 0xe6, 0x36, 0xcf, 0xeb, 0x31, 0xae,
	}
	GENUINE_FP_KEY = []byte{
		0x47, 0x65, 0x6E, 0x75, 0x69, 0x6E, 0x65, 0x20,
		0x41, 0x64, 0x6F, 0x62, 0x65, 0x20, 0x46, 0x6C,
		0x61, 0x73, 0x68, 0x20, 0x50, 0x6C, 0x61, 0x79,
		0x65, 0x72, 0x20, 0x30, 0x30, 0x31, /* Genuine Adobe Flash Player 001 */
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8,
		0x2E, 0x00, 0xD0, 0xD1, 0x02, 0x9E, 0x7E, 0x57,
		0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
)

func HMACSha256(msg []byte, key []byte) ([]byte, error) {
	h := hmac.New(sha256.New, key)
	if _, err := h.Write(msg); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func CreateRandomBlock(size uint) []byte {
	size64 := size / uint(8)
	var buf bytes.Buffer
	var r64 int64
	var i uint
	for i = uint(0); i < size64; i++ {
		r64 = rand.Int63()
		binary.Write(&buf, binary.BigEndian, &r64)
	}
	for i = i * uint(8); i < size; i++ {
		buf.WriteByte(byte(rand.Int()))
	}
	return buf.Bytes()
}

func CalcDigestPos(buf []byte, offset uint32, mod_val uint32, add_val uint32) (digest_pos uint32) {
	var i uint32
	for i = 0; i < 4; i++ {
		digest_pos += uint32(buf[i+offset])
	}
	digest_pos = digest_pos%mod_val + add_val
	return
}

func ValidateDigest(buf, key []byte, offset uint32) uint32 {
	digestPos := CalcDigestPos(buf, offset, 728, offset+4)
	var tmpBuf bytes.Buffer
	tmpBuf.Write(buf[:digestPos])
	tmpBuf.Write(buf[digestPos+SHA256_DIGEST_LENGTH:])
	tmpHash, err := HMACSha256(tmpBuf.Bytes(), key)
	if err != nil {
		return 0
	}
	if bytes.Compare(tmpHash, buf[digestPos:digestPos+SHA256_DIGEST_LENGTH]) == 0 {
		return digestPos
	}
	return 0
}

func ImprintWithDigest(buf, key []byte) uint32 {
	digestPos := CalcDigestPos(buf, 8, 728, 12)
	var tmpBuf bytes.Buffer
	tmpBuf.Write(buf[:digestPos])
	tmpBuf.Write(buf[digestPos+SHA256_DIGEST_LENGTH:])
	tmpHash, err := HMACSha256(tmpBuf.Bytes(), key)
	if err != nil {
		return 0
	}
	for idx, b := range tmpHash {
		buf[digestPos+uint32(idx)] = b
	}

	return digestPos
}

func SHandshake(c net.Conn, br *bufio.Reader, bw *bufio.Writer, timeout time.Duration) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()

	// Read C0
	if timeout > 0 {
		c.SetReadDeadline(time.Now().Add(timeout))
	}
	c0, err := br.ReadByte()
	errPanic(err, "SHandshake Read C0")
	if c0 != 0x03 {
		return fmt.Errorf("SHandshake Get C0: %x", c0)
	}

	// Read C1
	c1 := make([]byte, SIG_SIZE)
	if timeout > 0 {
		c.SetReadDeadline(time.Now().Add(timeout))
	}
	_, err = io.ReadAtLeast(br, c1, SIG_SIZE)
	errPanic(err, "SHandshake Read C1")
	logger.Debugf("SHandshake type version: %d.%d.%d.%d", c1[4], c1[5], c1[6], c1[7])

	// Send S0
	errPanic(bw.WriteByte(0x03), "SHandshake Send S0")

	// Send S1
	var clientDigestOffset uint32
	var isSimple bool = false
	if c1[4] == 0 && c1[5] == 0 && c1[6] == 0 && c1[7] == 0 {
		// simple
		_, err = bw.Write(c1)
		isSimple = true
		errPanic(err, "[simple] SHandshake Send S1")
	} else {
		scheme := 0
		clientDigestOffset = ValidateDigest(c1, GENUINE_FP_KEY[:30], 8)
		if clientDigestOffset == 0 {
			clientDigestOffset = ValidateDigest(c1, GENUINE_FP_KEY[:30], 772)
			if clientDigestOffset == 0 {
				return fmt.Errorf("SHandshake C1 validating failed")
			}
			scheme = 1
		}
		logger.Debugf("SHandshake scheme = %d", scheme)

		s1 := CreateRandomBlock(SIG_SIZE)
		binary.BigEndian.PutUint32(s1, uint32(0))
		for i := 0; i < 4; i++ {
			s1[4+i] = FMS_VERSION[i]
		}
		serverDigestOffset := ImprintWithDigest(s1, GENUINE_FMS_KEY[:36])
		if serverDigestOffset == 0 {
			return fmt.Errorf("ImprintWithDigest failed")
		}
		_, err = bw.Write(s1)
		errPanic(err, "[complex] SHandshake Send S1")
	}
	if timeout > 0 {
		c.SetWriteDeadline(time.Now().Add(timeout))
	}
	errPanic(bw.Flush(), "SHandshake Flush S0+S1")

	digestResp, err := HMACSha256(c1[clientDigestOffset:clientDigestOffset+SHA256_DIGEST_LENGTH], GENUINE_FMS_KEY)
	errPanic(err, "SHandshake Generate DigestResp")

	var s2 []byte
	// Generate S2
	if isSimple {
		s2 = c1
	} else {
		s2 = CreateRandomBlock(SIG_SIZE)
		signatureResp, err := HMACSha256(s2[:SIG_SIZE-SHA256_DIGEST_LENGTH], digestResp)
		errPanic(err, "SHandshake Generate S2 HMACSha256 signature")
		for idx, b := range signatureResp {
			s2[SIG_SIZE-SHA256_DIGEST_LENGTH+idx] = b
		}
	}

	// Send S2
	_, err = bw.Write(s2)
	errPanic(err, "SHandshake Send S2")
	if timeout > 0 {
		c.SetWriteDeadline(time.Now().Add(timeout))
	}
	errPanic(bw.Flush(), "SHandshake Flush S2")

	// Read C2
	if timeout > 0 {
		c.SetReadDeadline(time.Now().Add(timeout))
	}
	c2 := make([]byte, SIG_SIZE)
	_, err = io.ReadAtLeast(br, c2, SIG_SIZE)
	errPanic(err, "SHandshake Read C2")

	if timeout > 0 {
		c.SetDeadline(time.Time{})
	}

	return
}

func Handshake(c net.Conn, br *bufio.Reader, bw *bufio.Writer, timeout time.Duration) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()

	// Send C0 + C1
	errPanic(bw.WriteByte(0x03), "Handshake Send C0")
	c1 := CreateRandomBlock(SIG_SIZE)
	binary.BigEndian.PutUint32(c1, uint32(0))
	for i := 0; i < 4; i++ {
		c1[4+i] = FLASH_PLAYER_VERSION[i]
	}

	// TODO: Create the DH public/private key, and use it in encryption mode

	clientDigestOffset := ImprintWithDigest(c1, GENUINE_FP_KEY[:30])
	if clientDigestOffset == 0 {
		return fmt.Errorf("ImprintWithDigest failed")
	}
	_, err = bw.Write(c1)
	errPanic(err, "Handshake Send C1")
	if timeout > 0 {
		c.SetWriteDeadline(time.Now().Add(timeout))
	}
	errPanic(bw.Flush(), "Handshake Flush C0+C1")

	// Read S0
	if timeout > 0 {
		c.SetReadDeadline(time.Now().Add(timeout))
	}
	s0, err := br.ReadByte()
	errPanic(err, "Handshake Read S0")
	if s0 != 0x03 {
		return fmt.Errorf("Handshake Get s0: %x", s0)
	}

	// Read S1
	s1 := make([]byte, SIG_SIZE)
	if timeout > 0 {
		c.SetReadDeadline(time.Now().Add(timeout))
	}
	_, err = io.ReadAtLeast(br, s1, SIG_SIZE)
	errPanic(err, "Handshake Read S1")

	// Read S2
	if timeout > 0 {
		c.SetReadDeadline(time.Now().Add(timeout))
	}
	s2 := make([]byte, SIG_SIZE)
	_, err = io.ReadAtLeast(br, s2, SIG_SIZE)
	errPanic(err, "Handshake Read S2")

	server_pos := ValidateDigest(s1, GENUINE_FMS_KEY[:36], 8)
	if server_pos == 0 {
		server_pos = ValidateDigest(s1, GENUINE_FMS_KEY[:36], 772)
		if server_pos == 0 {
			return fmt.Errorf("Server response validating failed")
		}
	}

	digest, err := HMACSha256(c1[clientDigestOffset:clientDigestOffset+SHA256_DIGEST_LENGTH], GENUINE_FMS_KEY)
	errPanic(err, "Get digest from c1 error")

	signature, err := HMACSha256(s2[:SIG_SIZE-SHA256_DIGEST_LENGTH], digest)
	errPanic(err, "Get signature from s2 error")

	if bytes.Compare(signature, s2[SIG_SIZE-SHA256_DIGEST_LENGTH:]) != 0 {
		return fmt.Errorf("Server signature mismatch")
	}

	// Generate C2
	digestResp, err := HMACSha256(s1[server_pos:server_pos+SHA256_DIGEST_LENGTH], GENUINE_FP_KEY)
	errPanic(err, "Generate C2 HMACSha256 digestResp")

	c2 := CreateRandomBlock(SIG_SIZE)
	signatureResp, err := HMACSha256(c2[:SIG_SIZE-SHA256_DIGEST_LENGTH], digestResp)
	errPanic(err, "Generate C2 HMACSha256 signatureResp")
	for idx, b := range signatureResp {
		c2[SIG_SIZE-SHA256_DIGEST_LENGTH+idx] = b
	}

	// Send C2
	_, err = bw.Write(c2)
	errPanic(err, "Handshake Send C2")
	if timeout > 0 {
		c.SetWriteDeadline(time.Now().Add(timeout))
	}
	errPanic(bw.Flush(), "Handshake Flush C2")
	if timeout > 0 {
		c.SetDeadline(time.Time{})
	}

	return
}
