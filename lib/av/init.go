package av

import (
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"ken/log"
)

var (
	logger  *zap.SugaredLogger
	logOnce sync.Once
	conf    *Config
)

func init() {
	conf = &Config{
		AudioOnlyGopDuration: 5000,
		DelayTime:            3000,
		RingSize:             1024,
		SessionTimeout:       60,
		Sync:                 300,
	}
}

func InitLog(level zapcore.Level) {
	logOnce.Do(func() {
		logger = log.New("rtmp", level)
		logger = logger.Desugar().WithOptions(zap.AddCaller()).Sugar()
	})
}
