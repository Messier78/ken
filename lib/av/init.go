package av

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"ken/log"
)

var (
	logger *zap.SugaredLogger
)

func init() {
	logger = log.New("av", zapcore.DebugLevel)
	logger = logger.Desugar().WithOptions(zap.AddCaller()).Sugar()
}
