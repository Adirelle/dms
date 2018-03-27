package logging

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// RootLoggerName is the name of the root logger
	RootLoggerName = Name("")
	// RootLoggerAlias is an alias of the root logger
	RootLoggerAlias = "all"
)

//===========================================================================
// Config
//===========================================================================

// Config holds the logging configuration and is used the build the Factory.
type Config struct {
	Level LoggerLevels
	Quiet bool
	Debug bool
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	c := Config{Level: make(LoggerLevels)}
	c.Level[RootLoggerName] = zap.InfoLevel
	return c
}

// Build creates the Logger Factory
func (c *Config) Build() *Factory {
	encConf := zap.NewProductionEncoderConfig()
	encConf.EncodeLevel = zapcore.CapitalLevelEncoder
	encConf.TimeKey = ""

	f := &Factory{Config: *c, loggers: make(map[Name]Logger)}

	if c.Debug {
		f.options = append(f.options, zap.Development(), zap.AddCaller())
	}
	consoleEnc := zapcore.NewConsoleEncoder(encConf)

	f.cores = append(
		f.cores,
		zapcore.NewCore(consoleEnc, zapcore.AddSync(os.Stderr), zap.ErrorLevel),
	)
	if !c.Quiet {
		f.cores = append(
			f.cores,
			zapcore.NewCore(consoleEnc, zapcore.AddSync(os.Stdout), not{zap.ErrorLevel}),
		)
	}

	zLogger := f.Get(RootLoggerAlias).(*logger).SugaredLogger.Desugar()
	zap.ReplaceGlobals(zLogger)
	zap.RedirectStdLog(zLogger)
	return f
}

//===========================================================================
// Name
//===========================================================================

// Name is a clean, full Logger name
type Name string

// Clean creates a Logger Name from a string
func Clean(name string) Name {
	name = strings.Join(strings.Split(strings.Trim(name, "."), "."), ".")
	if name == RootLoggerAlias {
		return RootLoggerName
	}
	return Name(name)
}

// String implements fmt.Stringer
func (n Name) String() string {
	return string(n)
}

// Parent returns the full Name of the parent Logger.
func (n Name) Parent() Name {
	dot := strings.LastIndex(string(n), ".")
	if dot < 1 {
		return RootLoggerName
	}
	return Name(n[:dot])
}

// Child returns the full Name of a child Logger.
func (n Name) Child(s string) Name {
	if s == "" {
		return n
	}
	return Name(n.String() + "." + s)
}

//===========================================================================
// not
//===========================================================================

type not struct{ zapcore.LevelEnabler }

func (n not) Enabled(l zapcore.Level) bool {
	return !n.LevelEnabler.Enabled(l)
}

//===========================================================================
// LoggerLevels
//===========================================================================

// LoggerLevels is a map of Levels for Logger Names
type LoggerLevels map[Name]zapcore.Level

// Get implements flags.Getter
func (l LoggerLevels) Get() interface{} {
	return l
}

// Get implements fmt.Stringer
func (l LoggerLevels) String() string {
	b := &bytes.Buffer{}
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

// Set implements flags.Value. It parses a comma-separater strings of name:level couples.
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

// Resolve returns the Level to use for the Named Logger.
func (l LoggerLevels) Resolve(name Name) zapcore.Level {
	for cur := name; cur != RootLoggerName; cur = cur.Parent() {
		if level, found := l[cur]; found {
			return level
		}
	}
	return l[RootLoggerName]
}
