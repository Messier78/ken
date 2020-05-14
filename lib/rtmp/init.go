package rtmp

import (
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"ken/log"
)

var (
	logger  *zap.SugaredLogger
	logOnce sync.Once
)

func InitLog(level zapcore.Level) {
	logOnce.Do(func() {
		logger = log.New("rtmp", level)
		// logger = logger.Desugar().WithOptions(zap.AddCaller()).Sugar()
	})
}
