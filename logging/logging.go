package logging

import (
	"log"

	"go.uber.org/zap"
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
	Sync()
}

func NewDevelopment() Logger {
	l, err := zap.NewDevelopment()
	if err != nil {
		log.Panicf("could not create the logger: %s", err.Error())
	}
	return &wrapper{*l.Sugar()}
}

type wrapper struct{ zap.SugaredLogger }

func (w *wrapper) Named(name string) Logger {
	return &wrapper{*w.SugaredLogger.Named(name)}
}

func (w *wrapper) With(args ...interface{}) Logger {
	return &wrapper{*w.SugaredLogger.With(args...)}
}

func (w *wrapper) Sync() {
	err := w.SugaredLogger.Sync()
	if err != nil {
		log.Print(err)
	}
}
