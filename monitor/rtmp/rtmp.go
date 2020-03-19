package rtmp

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zapcore"

	"ken/log"
)

var (
	logger = log.New("[rtmp]", zapcore.DebugLevel)
)

func HandleMonitor(c *gin.Context) {
	logger.Debug("request: ", c.Request.URL.String())
	c.String(200, `-- KEN --
	HandleMonitor
`)
}
