package logging

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is a logger object..
type Logger interface {
	DPanic(...interface{})
	DPanicf(string, ...interface{})
	DPanicw(string, ...interface{})

	Debug(...interface{})
	Debugf(string, ...interface{})
	Debugw(string, ...interface{})

	Error(...interface{})
	Errorf(string, ...interface{})
	Errorw(string, ...interface{})

	Fatal(...interface{})
	Fatalf(string, ...interface{})
	Fatalw(string, ...interface{})

	Info(...interface{})
	Infof(string, ...interface{})
	Infow(string, ...interface{})

	Panic(...interface{})
	Panicf(string, ...interface{})
	Panicw(string, ...interface{})

	Warn(...interface{})
	Warnf(string, ...interface{})
	Warnw(string, ...interface{})

	Named(string) Logger
	With(...interface{}) Logger
	Sync() error
}

type Config struct {
	Debug       bool
	Level       zapcore.Level
	OutputPaths []string
}

func DefaultConfig() Config {
	return Config{
		Debug:       false,
		Level:       zap.InfoLevel,
		OutputPaths: []string{"stderr"},
	}
}

func New(c Config) Logger {
	var zConf zap.Config
	if c.Debug {
		zConf = zap.NewDevelopmentConfig()
		c.Level = zap.DebugLevel
		c.OutputPaths = []string{"stderr"}
	} else {
		zConf = zap.NewProductionConfig()
		zConf.DisableCaller = true
		zConf.DisableStacktrace = true
	}
	zConf.Level = zap.NewAtomicLevelAt(c.Level)
	zConf.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zConf.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	zConf.OutputPaths = c.OutputPaths
	zConf.Encoding = "console"
	logger := Wrap(zConf.Build())
	zap.RedirectStdLog(logger.(*wrapper).SugaredLogger.Desugar())
	return logger
}

func Wrap(logger interface{}, err error) Logger {
	if err == nil {
		switch wrapped := logger.(type) {
		case *zap.Logger:
			return &wrapper{*wrapped.Sugar()}
		case *zap.SugaredLogger:
			return &wrapper{*wrapped}
		}
		err = fmt.Errorf("Unknown logger type: %T", logger)
	}
	panic(err.Error())
}

type wrapper struct{ zap.SugaredLogger }

func (w *wrapper) Named(name string) Logger {
	return &wrapper{*w.SugaredLogger.Named(name)}
}

func (w *wrapper) With(args ...interface{}) Logger {
	return &wrapper{*w.SugaredLogger.With(args...)}
}

func (w *wrapper) Sync() error {
	return w.SugaredLogger.Sync()
}
