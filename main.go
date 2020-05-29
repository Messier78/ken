package main

import (
	"runtime"

	"go.uber.org/zap/zapcore"

	"ken/command"
	"ken/lib/av"
	"ken/lib/rtmp"
	"ken/log"
)

func main() {
	runtime.GOMAXPROCS(0)
	av.InitLog(zapcore.DebugLevel)
	rtmp.InitLog(zapcore.DebugLevel)
	command.Execute()

	log.Flush()
}
