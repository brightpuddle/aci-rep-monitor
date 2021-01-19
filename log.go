package main

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/mattn/go-colorable"
	"github.com/rs/zerolog"
)

// TODO syslog

var logMux sync.Mutex

type multiLevelWriter struct {
	file    io.Writer
	console io.Writer
}

func (w multiLevelWriter) Write(p []byte) (int, error) {
	logMux.Lock()
	count, err := w.file.Write(p)
	logMux.Unlock()
	return count, err
}

func (w multiLevelWriter) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if level >= zerolog.InfoLevel {
		n, err := w.console.Write(p)
		if err != nil {
			return n, err
		}
	}
	return w.file.Write(p)
}

func newLogger(logFile string) zerolog.Logger {
	file, err := os.Create(logFile)
	if err != nil {
		panic(fmt.Sprintf("cannot create log file %s", logFile))
	}

	// zerolog.SetGlobalLevel(zerolog.DebugLevel)
	zerolog.DurationFieldInteger = true

	writer := multiLevelWriter{
		file:    file,
		console: zerolog.ConsoleWriter{Out: colorable.NewColorableStdout()},
	}
	return zerolog.New(writer).With().Timestamp().Logger()
}
