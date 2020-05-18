package http

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zapcore"
)

// StartServer
func StartServer(bindAddress string) error {
	InitLog(zapcore.DebugLevel)
	r := gin.Default()
	h := &stream{}
	r.Any("/", gin.WrapH(h))
	return r.Run(bindAddress)
}
