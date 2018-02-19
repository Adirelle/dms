package logging

import (
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

//===========================================================================
// Factory
//===========================================================================

type Factory struct {
	Config
	baseCore zapcore.Core
	loggers  map[Name]Logger
	mu       sync.Mutex
}

func (f *Factory) Get(s string) Logger {
	return f.get(Clean(s))
}

func (f *Factory) get(name Name) Logger {
	f.mu.Lock()
	defer f.mu.Unlock()
	if logger, exists := f.loggers[name]; exists {
		return logger
	}
	level := f.Level.Resolve(name)
	core := &leveledCore{level, f.baseCore}
	zLogger := zap.New(core).Named(name.String())
	logger := &logger{f, name, zLogger.Sugar()}
	f.loggers[name] = logger
	return logger
}

//===========================================================================
// leveledCore
//===========================================================================

type leveledCore struct {
	zapcore.LevelEnabler
	zapcore.Core
}

func (c *leveledCore) Enabled(l zapcore.Level) bool {
	return c.LevelEnabler.Enabled(l)
}

func (c *leveledCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *leveledCore) With(fields []zapcore.Field) zapcore.Core {
	return &leveledCore{c.LevelEnabler, c.Core.With(fields)}
}
