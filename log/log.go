package log

import (
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	set "github.com/deckarep/golang-set"
)

var (
	logsLevel   sync.Map
	logs        = set.NewSet()
	once        sync.Once
	hookNormal  *lumberjack.Logger
	encoderConf *zapcore.EncoderConfig
)

func Rotate() {
	if hookNormal != nil {
		hookNormal.Rotate()
	}
}

func timeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05"))
}

// return logger wrote in normal.log
func New(tag string, level zapcore.Level) *zap.SugaredLogger {
	once.Do(func() {
		dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			panic(err)
		}
		filename := path.Join(dir, "logs/normal.log")
		hookNormal = &lumberjack.Logger{
			Filename:  filename,
			MaxSize:   500,
			LocalTime: true,
		}

		encoderConf = &zapcore.EncoderConfig{
			MessageKey:     "msg",
			LevelKey:       "level",
			TimeKey:        "time",
			NameKey:        "logger",
			CallerKey:      "caller",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseColorLevelEncoder,
			EncodeTime:     timeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
			EncodeName:     zapcore.FullNameEncoder,
		}
	})
	var atomicLevel zap.AtomicLevel

	if alvl, ok := logsLevel.Load(tag); ok {
		atomicLevel, _ = alvl.(zap.AtomicLevel)
	} else {
		atomicLevel = zap.NewAtomicLevel()
		logsLevel.Store(tag, atomicLevel)
	}

	atomicLevel.SetLevel(level)
	var syncer zapcore.WriteSyncer
	if level == zapcore.DebugLevel {
		syncer = zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout),
			zapcore.AddSync(hookNormal))
	} else {
		syncer = zapcore.AddSync(hookNormal)
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(*encoderConf),
		// zapcore.AddSync(hookNormal),
		syncer,
		atomicLevel,
	)

	ls := zap.New(core).WithOptions(zap.AddCaller()).Sugar().Named(tag)
	logs.Add(ls)
	return ls
}

// ResetLevel
func ResetLevel(tag string, level zapcore.Level) {
	if v, ok := logsLevel.Load(tag); ok {
		if atomicLevel, ok := v.(zap.AtomicLevel); ok {
			atomicLevel.SetLevel(level)
		}
	}
}

// flush
func Flush() {
	logs.Each(func(v interface{}) bool {
		if logger, ok := v.(*zap.SugaredLogger); ok {
			logger.Sync()
		}
		return false
	})
}
