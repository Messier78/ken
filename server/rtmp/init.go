package rtmp

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"ken/log"
)

var (
	logger *zap.SugaredLogger
)

func init() {
	log.New("rtmp", zapcore.DebugLevel)
}
