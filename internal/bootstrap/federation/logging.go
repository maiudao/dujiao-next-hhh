package federation

import (
	"context"
	"log/slog"

	"go.uber.org/zap"
)

// Logger gives the library and the browser adapter the same restricted sink.
// Library errors can contain protocol input; preserve classification and the
// request ID, never arbitrary messages, URLs, error strings or structured data.
func Logger(log *zap.Logger, requestID func(context.Context) string) *slog.Logger {
	if log == nil {
		log = zap.NewNop()
	}
	return slog.New(safeHandler{log: log, requestID: requestID})
}

type safeHandler struct {
	log       *zap.Logger
	requestID func(context.Context) string
}

func (h safeHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h safeHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h safeHandler) WithGroup(string) slog.Handler            { return h }
func (h safeHandler) Handle(ctx context.Context, record slog.Record) error {
	message := "identity protocol event"
	switch record.Message {
	case "identity request rejected", "identity authentication failed", "identity authentication completed":
		message = record.Message
	}
	fields := make([]zap.Field, 0, 5)
	if h.requestID != nil {
		fields = append(fields, zap.String("request_id", h.requestID(ctx)))
	}
	record.Attrs(func(attr slog.Attr) bool {
		// Only fixed adapter messages may carry their known metadata.
		if message == "identity protocol event" {
			return false
		}
		switch attr.Key {
		case "action", "client":
			if attr.Value.Kind() == slog.KindString {
				fields = append(fields, zap.String(attr.Key, attr.Value.String()))
			}
		case "status", "user_id":
			if attr.Value.Kind() == slog.KindInt64 {
				fields = append(fields, zap.Int64(attr.Key, attr.Value.Int64()))
			} else if attr.Value.Kind() == slog.KindUint64 {
				fields = append(fields, zap.Uint64(attr.Key, attr.Value.Uint64()))
			}
		}
		return true
	})
	if record.Level >= slog.LevelError {
		h.log.Error(message, fields...)
	} else if record.Level >= slog.LevelWarn {
		h.log.Warn(message, fields...)
	} else {
		h.log.Info(message, fields...)
	}
	return nil
}
