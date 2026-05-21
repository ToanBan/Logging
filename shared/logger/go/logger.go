package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

type LogContext struct {
	ReqID   string
	TraceID string
	UserID  string
	Extra   map[string]any
}

type Logger struct {
	service string
	zl      zerolog.Logger
}

func NewLogger(service string) *Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "timestamp"
	zerolog.LevelFieldName = "level"
	zerolog.MessageFieldName = "msg"

	zl := zerolog.New(os.Stdout).With().Timestamp().Logger()

	return &Logger{service: service, zl: zl}
}

func (l *Logger) buildEvent(event *zerolog.Event, ctx LogContext) *zerolog.Event {
	event = event.Str("service", l.service)

	if ctx.ReqID != "" {
		event = event.Str("req_id", ctx.ReqID)
	}
	if ctx.TraceID != "" {
		event = event.Str("trace_id", ctx.TraceID)
	}
	if ctx.UserID != "" {
		event = event.Str("user_id", ctx.UserID)
	}
	if len(ctx.Extra) > 0 {
		event = event.Interface("extra", ctx.Extra)
	}

	return event
}

func (l *Logger) Debug(msg string, ctx LogContext) {
	l.buildEvent(l.zl.Debug(), ctx).Msg(msg)
}

func (l *Logger) Info(msg string, ctx LogContext) {
	l.buildEvent(l.zl.Info(), ctx).Msg(msg)
}

func (l *Logger) Warn(msg string, ctx LogContext) {
	l.buildEvent(l.zl.Warn(), ctx).Msg(msg)
}

func (l *Logger) Error(msg string, ctx LogContext) {
	l.buildEvent(l.zl.Error(), ctx).Msg(msg)
}

func (l *Logger) Fatal(msg string, ctx LogContext) {
	l.buildEvent(l.zl.Fatal(), ctx).Msg(msg)
}
