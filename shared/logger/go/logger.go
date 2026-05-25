package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

type LogContext struct {
	ReqID   string
	TraceID string
	UserID  string
	Extra   map[string]any
}

type LogEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Service   string         `json:"service"`
	ReqID     string         `json:"req_id,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
	UserID    string         `json:"user_id,omitempty"`
	Msg       string         `json:"msg"`
	Extra     map[string]any `json:"extra,omitempty"`
}

var logFile *os.File

func initLogFile() io.Writer {

	dir := "/shared/logger/logs"

	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"[Logger Init] failed to create log dir: %v\n",
			err,
		)

		return os.Stdout
	}

	path := dir + "/combined.log"

	f, err := os.OpenFile(
		path,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)

	if err != nil {

		fmt.Fprintf(
			os.Stderr,
			"[Logger Init] failed to open log file: %v\n",
			err,
		)

		return os.Stdout
	}

	fmt.Fprintf(
		os.Stderr,
		"[Logger Init] LOGS_DIRECTORY: %s\n",
		dir,
	)

	fmt.Fprintf(
		os.Stderr,
		"[Logger Init] COMBINED_LOG_PATH: %s\n",
		path,
	)

	logFile = f

	return f
}

var (
	kafkaWriter     *kafka.Writer
	kafkaWriterOnce sync.Once
)

func ensureTopic(broker string) {

	conn, err := kafka.Dial("tcp", broker)

	if err != nil {

		fmt.Fprintf(
			os.Stderr,
			"[kafka-init-error] dial: %v\n",
			err,
		)

		return
	}

	defer conn.Close()

	controller, err := conn.Controller()

	if err != nil {

		fmt.Fprintf(
			os.Stderr,
			"[kafka-init-error] controller: %v\n",
			err,
		)

		return
	}

	controllerConn, err := kafka.Dial(
		"tcp",
		net.JoinHostPort(
			controller.Host,
			fmt.Sprintf("%d", controller.Port),
		),
	)

	if err != nil {

		fmt.Fprintf(
			os.Stderr,
			"[kafka-init-error] controller dial: %v\n",
			err,
		)

		return
	}

	defer controllerConn.Close()

	err = controllerConn.CreateTopics(
		kafka.TopicConfig{
			Topic:             "req-logs",
			NumPartitions:     1,
			ReplicationFactor: 1,
		},
	)

	if err != nil &&
		!strings.Contains(
			err.Error(),
			"Topic with this name already exists",
		) {

		fmt.Fprintf(
			os.Stderr,
			"[kafka-init-error] create topic: %v\n",
			err,
		)
	}
}

func getKafkaWriter() *kafka.Writer {

	kafkaWriterOnce.Do(func() {

		broker := os.Getenv("KAFKA_BROKER")

		if broker == "" {
			broker = "kafka:9092"
		}

		ensureTopic(broker)

		kafkaWriter = &kafka.Writer{
			Addr:         kafka.TCP(broker),
			Topic:        "req-logs",
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireAll,
			Async:        false,
		}
	})

	return kafkaWriter
}

func flushToKafka(entries []LogEntry) {

	if len(entries) == 0 {
		return
	}

	data, err := json.Marshal(entries)

	if err != nil {

		fmt.Fprintf(
			os.Stderr,
			"[kafka-flush-error] marshal: %v\n",
			err,
		)

		return
	}

	w := getKafkaWriter()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)

	defer cancel()

	err = w.WriteMessages(
		ctx,
		kafka.Message{
			Value: data,
		},
	)

	if err != nil {

		fmt.Fprintf(
			os.Stderr,
			"[kafka-flush-error] write: %v\n",
			err,
		)

		return
	}
}

const bufferSize = 64

type RingBuffer struct {
	buffer []LogEntry
}

func (r *RingBuffer) push(entry LogEntry) {

	if len(r.buffer) >= bufferSize {
		r.buffer = r.buffer[1:]
	}

	r.buffer = append(r.buffer, entry)
}

func (r *RingBuffer) flush() []LogEntry {

	cp := make([]LogEntry, len(r.buffer))

	copy(cp, r.buffer)

	return cp
}

func (r *RingBuffer) clear() {
	r.buffer = []LogEntry{}
}

func buildEntry(
	service,
	level,
	msg string,
	ctx LogContext,
) LogEntry {

	return LogEntry{
		Timestamp: time.Now().
			UTC().
			Format(time.RFC3339Nano),

		Level:   level,
		Service: service,
		ReqID:   ctx.ReqID,
		TraceID: ctx.TraceID,
		UserID:  ctx.UserID,
		Msg:     msg,
		Extra:   ctx.Extra,
	}
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

	writer := initLogFile()

	zl := zerolog.
		New(writer).
		With().
		Timestamp().
		Logger()

	return &Logger{
		service: service,
		zl:      zl,
	}
}

func (l *Logger) buildEvent(
	event *zerolog.Event,
	ctx LogContext,
) *zerolog.Event {

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

func (l *Logger) Done() {}

type RequestLogger struct {
	service string
	reqID   string
	ring    RingBuffer
}

func NewRequestLogger(
	service,
	reqID string,
) *RequestLogger {

	return &RequestLogger{
		service: service,
		reqID:   reqID,
	}
}

func (r *RequestLogger) log(
	level,
	msg string,
	ctx LogContext,
) {

	ctx.ReqID = r.reqID

	r.ring.push(
		buildEntry(
			r.service,
			level,
			msg,
			ctx,
		),
	)
}

func (r *RequestLogger) flush() {

	entries := r.ring.flush()

	flushToKafka(entries)

	r.ring.clear()
}

func (r *RequestLogger) Debug(
	msg string,
	ctx LogContext,
) {

	r.log("debug", msg, ctx)
}

func (r *RequestLogger) Info(
	msg string,
	ctx LogContext,
) {

	r.log("info", msg, ctx)
}

func (r *RequestLogger) Warn(
	msg string,
	ctx LogContext,
) {

	r.log("warn", msg, ctx)

	r.flush()
}

func (r *RequestLogger) Error(
	msg string,
	ctx LogContext,
) {

	r.log("error", msg, ctx)

	r.flush()
}

func (r *RequestLogger) Fatal(
	msg string,
	ctx LogContext,
) {

	r.log("fatal", msg, ctx)

	r.flush()
}

func (r *RequestLogger) Done() {
	r.ring.clear()
}