package logging

import (
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

//===========================================================================
// Factory
//===========================================================================

// Factory is used to build Loggers.
type Factory struct {
	Config
	cores   []zapcore.Core
	options []zap.Option
	loggers map[Name]Logger
	mu      sync.Mutex
}

// Get returns a Logger for the given name.
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
	core := &leveledCore{level, f.cores}
	zLogger := zap.New(core, f.options...).Named(name.String())
	logger := &logger{f, name, zLogger.Sugar()}
	f.loggers[name] = logger
	return logger
}

//===========================================================================
// leveledCore
//===========================================================================

type leveledCore struct {
	zapcore.LevelEnabler
	cores []zapcore.Core
}

func (c *leveledCore) Enabled(l zapcore.Level) bool {
	return c.LevelEnabler.Enabled(l)
}

func (c *leveledCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		for _, core := range c.cores {
			ce = core.Check(ent, ce)
		}
	}
	return ce
}

func (c *leveledCore) With(fields []zapcore.Field) zapcore.Core {
	cores := make([]zapcore.Core, len(c.cores))
	for i, core := range c.cores {
		cores[i] = core.With(fields)
	}
	return &leveledCore{c.LevelEnabler, cores}
}

func (c *leveledCore) Write(ent zapcore.Entry, fields []zapcore.Field) (err error) {
	for _, core := range c.cores {
		err = core.Write(ent, fields)
	}
	return
}

func (c *leveledCore) Sync() (err error) {
	for _, core := range c.cores {
		err = core.Sync()
	}
	return
}
