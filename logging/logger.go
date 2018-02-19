package logging

import (
	"io"

	"go.uber.org/zap"
)

//===========================================================================
// Logger
//===========================================================================

// Logger is a logger object
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

	Writer() io.WriteCloser
}

//===========================================================================
// logger
//===========================================================================

type logger struct {
	factory *Factory
	name    Name
	*zap.SugaredLogger
}

func (l *logger) Named(s string) Logger {
	return l.factory.get(l.name.Child(s))
}

func (l *logger) With(args ...interface{}) Logger {
	return &logger{l.factory, l.name, l.SugaredLogger.With(args...)}
}

func (l *logger) Sync() error {
	return l.SugaredLogger.Sync()
}

func (l *logger) Writer() io.WriteCloser {
	return &writer{l}
}

//===========================================================================
// writer
//===========================================================================

type writer struct {
	l Logger
}

func (w *writer) Write(b []byte) (int, error) {
	w.l.Info(b)
	return len(b), nil
}

func (w *writer) Close() error {
	return nil
}
