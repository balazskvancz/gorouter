package gorouter

import (
	"fmt"
	"log"
	"os"
)

type (
	logTypeName  string
	logTypeValue uint8
)

const (
	tInfo    logTypeName = "info"
	tWarning logTypeName = "warning"
	tError   logTypeName = "error"
)

const (
	InfoLogLevel logTypeValue = 1 << iota
	WarningLogLevel
	ErrorLogLevel
)

const defaultLogLevel = InfoLogLevel + WarningLogLevel + ErrorLogLevel

var logLevelValues = map[logTypeName]logTypeValue{
	tInfo:    InfoLogLevel,
	tWarning: WarningLogLevel,
	tError:   ErrorLogLevel,
}

type logger struct {
	logLevel logTypeValue
	*log.Logger
}

type Logger interface {
	Info(string, ...any)
	Error(string, ...any)
	Warning(string, ...any)
	// disable(logTypeValue)
}

const (
	defaultLogFlag int = log.LstdFlags
)

var _ Logger = (*logger)(nil)

func newLogger(serverName string) Logger {
	logPrefix := fmt.Sprintf("[%s %s] ", serverName, version)

	return &logger{
		Logger:   log.New(os.Stdout, logPrefix, defaultLogFlag),
		logLevel: defaultLogLevel,
	}
}

func (l *logger) Info(format string, v ...any) {
	l.write(tInfo, fmt.Sprintf("[INFO] – %s\n", format), v...)
}

func (l *logger) Error(format string, v ...any) {
	l.write(tError, fmt.Sprintf("[ERROR] – %s\n", format), v...)
}

func (l *logger) Warning(format string, v ...any) {
	l.write(tWarning, fmt.Sprintf("[WARNING] – %s\n", format), v...)
}

func (l *logger) write(logType logTypeName, format string, v ...any) {
	if (logLevelValues[logType] & l.logLevel) == 0 {
		return
	}
	l.Printf(format, v...)
}

// Maybe to it later.
// func (l *logger) disable(d logTypeValue) {
// if l.logLevel&d == 0 {
// return
// }
// l.logLevel -= d
// }
