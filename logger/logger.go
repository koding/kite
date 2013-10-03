package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// A Logger represents an object that generates lines of output to to an
// io.Writer. By default it uses os.Stdout, but it can be changed  or others
// may be included during creation.
type Logger struct {
	mu      sync.Mutex    // protects the following fields
	disable bool          // global switch to disable log completely
	prefix  func() string // function return is written at beginning of each line
	out     io.Writer     // destination for ouput
}

// New creates a new Logger. The filepath sets the files that will be used
// as an extra output destination. By default logger also outputs to stdout.
func New(filepath ...string) *Logger {
	writers := make([]io.Writer, 0)
	for _, path := range filepath {
		logFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0640)
		if err != nil {
			fmt.Printf("logger: can't open %s: '%s'\n", path, err)
			continue
		}

		writers = append(writers, logFile)
	}

	writers = append(writers, os.Stdout)

	return &Logger{
		out: io.MultiWriter(writers...),
		prefix: func() string {
			return fmt.Sprintf("[%s] ", time.Now().Format(time.Stamp))
		},
	}
}

// Print formats using the default formats for its operands and writes to
// standard output. Spaces are added between operands when neither is a string. It
// returns the number of bytes written and any write error encountered.
func (l *Logger) Printn(v ...interface{}) (int, error) {
	if l.debugEnabled() {
		return 0, nil
	}

	return fmt.Fprint(l.output(), v...)
}

// Printf formats according to a format specifier and writes to standard output.
// It returns the number of bytes written and any write error encountered.
func (l *Logger) Printf(format string, v ...interface{}) (int, error) {
	if l.debugEnabled() {
		return 0, nil
	}

	return fmt.Fprintf(l.output(), format, v...)
}

// Println formats using the default formats for its operands and writes to
// standard output. Spaces are always added between operands and a newline is
// appended. It returns the number of bytes written and any write error
// encountered.
func (l *Logger) Println(v ...interface{}) (int, error) {
	if l.debugEnabled() {
		return 0, nil
	}

	return fmt.Fprintln(l.output(), v...)
}

// SetPrefix sets the output prefix according to the return value of the passed
// function.
func (l *Logger) SetPrefix(fn func() string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prefix = fn
}

// Prefix returns the output prefix.
func (l *Logger) Prefix() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.prefix()
}

// SetOutput replaces the standard destination.
func (l *Logger) SetOutput(out io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = out
}

// DisableLog is a global switch that disables the output completely. Useful
// if you want turn off/on logs for debugging.
func (l *Logger) DisableLog() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.disable = true
}

func (l *Logger) output() io.Writer {
	l.out.Write([]byte(l.prefix()))
	return l.out
}

func (l *Logger) debugEnabled() bool {
	return l.disable
}
