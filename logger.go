package kite

import (
	"log"
	"os"
	"strings"

	"github.com/koding/logging"
)

type Level int

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
	Fatal(format string, args ...interface{})
	Error(format string, args ...interface{})
	Warning(format string, args ...interface{})
	Info(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

func newLogger() Logger {
	return log.New(os.Stderr, "kite", log.LstdFlags)
}

// getLogLevel returns the logging level defined via the KITE_LOG_LEVEL
// environment. It returns logging.Info by default if no environment variable
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
		return CRITICAL
	default:
		return INFO
	}
}

// newLogger returns a new logger object for desired name and level.
func newKodingLogger(name string) Logger {
	logger := logging.NewLogger(name)
	logger.SetLevel(getLogLevel())

	return logger
}
