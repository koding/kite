package logging

import (
	"bytes"
	"fmt"
	"io"
	"log/syslog"
	"os"
	"strings"
	"sync"
	"time"
)

type (
	color int
	level int
)

// Colors for different log levels.
const (
	Black color = (iota + 30)
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White
)

// Logging levels.
const (
	Critical level = iota
	Error
	Warning
	Notice
	Info
	Debug
)

// Logger is the interface for outputing log messages in different levels.
// A new Logger can be created with NewLogger() function.
// You can changed the output backend with SetBackend() function.
type Logger interface {
	// SetLevel changes the level of the logger. Default is logging.Info.
	SetLevel(level)

	// SetBackend replaces the current backend for output. Default is logging.StderrBackend.
	SetBackend(Backend)

	// Close backends.
	Close()

	// Fatal is equivalent to l.Critical followed by a call to os.Exit(1).
	Fatal(format string, args ...interface{})

	// Panic is equivalent to l.Critical followed by a call to panic().
	Panic(format string, args ...interface{})

	// Critical logs a message using CRITICAL as log level.
	Critical(format string, args ...interface{})

	// Error logs a message using ERROR as log level.
	Error(format string, args ...interface{})

	// Warning logs a message using WARNING as log level.
	Warning(format string, args ...interface{})

	// Notice logs a message using NOTICE as log level.
	Notice(format string, args ...interface{})

	// Info logs a message using INFO as log level.
	Info(format string, args ...interface{})

	// Debug logs a message using DEBUG as log level.
	Debug(format string, args ...interface{})
}

// Backend is the main component of Logger that handles the output.
type Backend interface {
	// Handles one log message.
	Log(name string, level string, color color, format string, args ...interface{})

	// Close the backend.
	Close()
}

///////////////////////////////////
//                               //
// Default Logger implementation //
//                               //
///////////////////////////////////

// logger is the default Logger implementation.
type logger struct {
	Name    string
	Level   level
	Backend Backend
}

// NewLogger returns a new Logger implementation. Do not forget to close it at exit.
func NewLogger(name string) Logger {
	return &logger{
		Name:    name,
		Level:   Info,
		Backend: StderrBackend,
	}
}

func (l *logger) Close() {
	l.Backend.Close()
}

func (l *logger) SetLevel(level level) {
	l.Level = level
}

func (l *logger) SetBackend(b Backend) {
	l.Backend = b
}

func (l *logger) log(level string, color color, format string, args ...interface{}) {
	// Add missing newline at the end.
	if !strings.HasSuffix(format, "\n") {
		format += "\n"
	}

	l.Backend.Log(l.Name, level, color, format, args...)
}

func (l *logger) Fatal(format string, args ...interface{}) {
	l.Critical(format, args...)
	l.Close()
	os.Exit(1)
}

func (l *logger) Panic(format string, args ...interface{}) {
	l.Critical(format, args...)
	l.Close()
	panic(fmt.Sprintf(format, args...))
}

func (l *logger) Critical(format string, args ...interface{}) {
	if l.Level >= Critical {
		l.log("CRITICAL", Magenta, format, args...)
	}
}

func (l *logger) Error(format string, args ...interface{}) {
	if l.Level >= Error {
		l.log("ERROR", Red, format, args...)
	}
}

func (l *logger) Warning(format string, args ...interface{}) {
	if l.Level >= Warning {
		l.log("WARNING", Yellow, format, args...)
	}
}

func (l *logger) Notice(format string, args ...interface{}) {
	if l.Level >= Notice {
		l.log("NOTICE", Green, format, args...)
	}
}

func (l *logger) Info(format string, args ...interface{}) {
	if l.Level >= Info {
		l.log("INFO", White, format, args...)
	}
}

func (l *logger) Debug(format string, args ...interface{}) {
	if l.Level >= Debug {
		l.log("DEBUG", Cyan, format, args...)
	}
}

///////////////////
//               //
// WriterBackend //
//               //
///////////////////

// WriterBackend is a backend implementation that writes the logging output to a io.Writer.
type WriterBackend struct {
	w io.Writer
}

func NewWriterBackend(w io.Writer) *WriterBackend {
	return &WriterBackend{w: w}
}

func (b *WriterBackend) Log(name string, level string, color color, format string, args ...interface{}) {
	fmt.Fprint(b.w, prefix(name, level)+colorize(fmt.Sprintf(format, args...), color))
}

func (b *WriterBackend) Close() {}

var StderrBackend = NewWriterBackend(os.Stderr)
var StdoutBackend = NewWriterBackend(os.Stdout)

///////////////////
//               //
// SyslogBackend //
//               //
///////////////////

// SyslogBackend sends the logging output to syslog.
type SyslogBackend struct {
	w *syslog.Writer
}

func NewSyslogBackend(tag string) (*SyslogBackend, error) {
	// Priority in New constructor is not important here because we
	// do not use w.Write() directly.
	w, err := syslog.New(syslog.LOG_INFO|syslog.LOG_USER, tag)
	if err != nil {
		return nil, err
	}
	return &SyslogBackend{w: w}, nil
}

func (b *SyslogBackend) Log(name string, level string, color color, format string, args ...interface{}) {
	var fn func(string) error
	switch level {
	case "CRITICAL":
		fn = b.w.Crit
	case "ERROR":
		fn = b.w.Err
	case "WARNING":
		fn = b.w.Warning
	case "NOTICE":
		fn = b.w.Notice
	case "INFO":
		fn = b.w.Info
	case "DEBUG":
		fn = b.w.Debug
	}
	fn(fmt.Sprintf(format, args...))
}

func (b *SyslogBackend) Close() {
	b.w.Close()
}

//////////////////
//              //
// MultiBackend //
//              //
//////////////////

// MultiBackend sends the log output to multiple backends concurrently.
type MultiBackend struct {
	backends []Backend
}

func NewMultiBackend(backends ...Backend) *MultiBackend {
	return &MultiBackend{backends: backends}
}

func (b *MultiBackend) Log(name string, level string, color color, format string, args ...interface{}) {
	wg := sync.WaitGroup{}
	wg.Add(len(b.backends))
	for _, backend := range b.backends {
		go func(backend Backend) {
			backend.Log(name, level, color, format, args...)
			wg.Done()
		}(backend)
	}
	wg.Wait()
}

func (b *MultiBackend) Close() {
	wg := sync.WaitGroup{}
	wg.Add(len(b.backends))
	for _, backend := range b.backends {
		go func(backend Backend) {
			backend.Close()
			wg.Done()
		}(backend)
	}
	wg.Wait()
}

///////////
//       //
// Utils //
//       //
///////////

func prefix(name, level string) string {
	return fmt.Sprintf("%s %s %-8s ", fmt.Sprint(time.Now().UTC())[:19], name, level)
}

func colorize(s string, color color) string {
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("\033[%dm", color))
	buf.WriteString(s)
	buf.WriteString("\033[0m") // reset color
	return buf.String()
}
