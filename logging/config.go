package logging

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	RootLoggerName  = Name("")
	RootLoggerAlias = "all"
)

//===========================================================================
// Config
//===========================================================================

type Config struct {
	Level LoggerLevels
	Quiet bool
}

func DefaultConfig() Config {
	c := Config{Level: make(LoggerLevels)}
	c.Level[RootLoggerName] = zap.InfoLevel
	return c
}

func (c *Config) Build() *Factory {

	encConf := zap.NewProductionEncoderConfig()
	encConf.EncodeLevel = zapcore.CapitalLevelEncoder
	encConf.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleEnc := zapcore.NewConsoleEncoder(encConf)

	cores := []zapcore.Core{
		zapcore.NewCore(consoleEnc, zapcore.AddSync(os.Stderr), zap.ErrorLevel),
	}
	if !c.Quiet {
		cores = append(
			cores,
			zapcore.NewCore(consoleEnc, zapcore.AddSync(os.Stdout), belowLevel{zap.ErrorLevel}),
		)
	}

	f := &Factory{
		Config:   *c,
		baseCore: zapcore.NewTee(cores...),
		loggers:  make(map[Name]Logger),
	}
	zLogger := f.Get(RootLoggerAlias).(*logger).SugaredLogger.Desugar()
	zap.ReplaceGlobals(zLogger)
	zap.RedirectStdLog(zLogger)
	return f
}

//===========================================================================
// Name
//===========================================================================

type Name string

func Clean(name string) Name {
	name = strings.Join(strings.Split(strings.Trim(name, "."), "."), ".")
	if name == RootLoggerAlias {
		return RootLoggerName
	}
	return Name(name)
}

func (n Name) String() string {
	return string(n)
}

func (n Name) Parent() Name {
	dot := strings.LastIndex(string(n), ".")
	if dot < 1 {
		return RootLoggerName
	}
	return Name(n[:dot])
}

func (n Name) Child(s string) Name {
	if s == "" {
		return n
	}
	return Name(n.String() + "." + s)
}

//===========================================================================
// belowLevel
//===========================================================================

type belowLevel struct{ zapcore.Level }

func (bl belowLevel) Enabled(l zapcore.Level) bool {
	return !bl.Level.Enabled(l)
}

//===========================================================================
// LoggerLevels
//===========================================================================

type LoggerLevels map[Name]zapcore.Level

func (l LoggerLevels) Get() interface{} {
	return l
}

func (l LoggerLevels) String() string {
	b := writerPool.Get()
	defer b.Free()
	first := true
	for k, v := range l {
		if first {
			first = false
		} else {
			fmt.Fprint(b, ",")
		}
		if k == "" {
			k = "all"
		}
		fmt.Fprintf(b, "%s:%s", k, v)
	}
	return b.String()
}

func (l LoggerLevels) Set(value string) (err error) {
	items := strings.Split(value, ",")
	for _, item := range items {
		var name, value string
		if parts := strings.SplitN(item, ":", 2); len(parts) == 1 {
			value = strings.Trim(parts[0], " ")
		} else {
			name = strings.Trim(parts[0], " ")
			value = strings.Trim(parts[1], " ")
		}
		lvl := zapcore.DebugLevel
		err = (&lvl).Set(value)
		if err != nil {
			return
		}
		l[Clean(name)] = lvl
	}
	return
}

func (l LoggerLevels) Resolve(name Name) zapcore.Level {
	for cur := name; cur != RootLoggerName; cur = cur.Parent() {
		if level, found := l[cur]; found {
			return level
		}
	}
	return l[RootLoggerName]
}
