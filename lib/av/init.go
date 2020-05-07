package av

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"ken/log"
)

var (
	logger *zap.SugaredLogger
	conf   *Config
)

func init() {
	logger = log.New("av", zapcore.DebugLevel)
	logger = logger.Desugar().WithOptions(zap.AddCaller()).Sugar()
	conf = &Config{
		AudioOnlyGopDuration: 5000,
		DelayTime:            3000,
		RingSize:             1024,
	}
}
