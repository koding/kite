package kite

import (
	"os"
	"strings"

	"github.com/koding/logging"
)

type Level int

var debugMode bool

// Logging levels.
const (
	FATAL Level = iota
	ERROR
	WARNING
	INFO
	DEBUG
)

// Logger is the interface used to log messages in different levels.
type Logger interface {
	// Fatal logs to the FATAL, ERROR, WARNING, INFO and DEBUG levels,
	// including a stack trace of all running goroutines, then calls
	// os.Exit(1).
	Fatal(format string, args ...interface{})

	// Error logs to the ERROR, WARNING, INFO and DEBUG level.
	Error(format string, args ...interface{})

	// Warning logs to the WARNING, INFO and DEBUG level.
	Warning(format string, args ...interface{})

	// Info logs to the INFO and DEBUG level.
	Info(format string, args ...interface{})

	// Debug logs to the DEBUG level.
	Debug(format string, args ...interface{})
}

// getLogLevel returns the logging level defined via the KITE_LOG_LEVEL
// environment. It returns Info by default if no environment variable
// is set.
func getLogLevel() Level {
	switch strings.ToUpper(os.Getenv("KITE_LOG_LEVEL")) {
	case "DEBUG":
		return DEBUG
	case "WARNING":
		return WARNING
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO
	}
}

// convertLevel converts a kite level into logging level
func convertLevel(l Level) logging.Level {
	switch l {
	case DEBUG:
		return logging.DEBUG
	case WARNING:
		return logging.WARNING
	case ERROR:
		return logging.ERROR
	case FATAL:
		return logging.CRITICAL
	default:
		return logging.INFO
	}
}

// newLogger returns a new kite logger based on koding/logging package and a
// SetLogLvel function. The current logLevel is INFO by default, which can be
// changed with KITE_LOG_LEVEL environment variable.
func newLogger(name string) (Logger, func(Level)) {
	logger := logging.NewLogger(name)
	logger.SetLevel(convertLevel(getLogLevel()))

	if os.Getenv("KITE_LOG_NOCOLOR") != "" {
		logging.StdoutHandler.Colorize = false
		logging.StderrHandler.Colorize = false
	}

	setLevel := func(l Level) {
		logger.SetLevel(convertLevel(l))
		logging.DefaultHandler.SetLevel(convertLevel(l))
	}

	return logger, setLevel
}
