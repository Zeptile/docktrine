package logger

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Init() {
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
		NoColor:    false,
	}

	log.Logger = zerolog.New(output).
		With().
		Timestamp().
		Logger()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}


func Info(msg string) {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
		line = 0
	}
	log.Info().Str("caller", fmt.Sprintf("%s:%d", file, line)).Msg(msg)
}

func Error(err error, msg string) {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
		line = 0
	}
	log.Error().Err(err).Str("caller", fmt.Sprintf("%s:%d", file, line)).Msg(msg)
}

func Debug(msg string) {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
		line = 0
	}
	log.Debug().Str("caller", fmt.Sprintf("%s:%d", file, line)).Msg(msg)
}

func Warn(msg string) {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
		line = 0
	}
	log.Warn().Str("caller", fmt.Sprintf("%s:%d", file, line)).Msg(msg)
}

func Fatal(err error, msg string) {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
		line = 0
	}
	log.Fatal().Err(err).Str("caller", fmt.Sprintf("%s:%d", file, line)).Msg(msg)
} 