package logging

import (
	"errors"
	"fmt"
	"syscall"

	"go.uber.org/zap"
)

type Field struct {
	Key   string
	Value any
}

func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

func Error(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: "<nil>"}
	}
	return Field{Key: "error", Value: err.Error()}
}

type Logger interface {
	Info(message string, fields ...Field) error
	Error(message string, fields ...Field) error
	Sync() error
}

type zapLogger struct {
	logger *zap.Logger
}

func New() (Logger, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("create zap logger: %w", err)
	}
	return zapLogger{logger: logger}, nil
}

func (l zapLogger) Info(message string, fields ...Field) error {
	l.logger.Info(message, toZapFields(fields)...)
	return nil
}

func (l zapLogger) Error(message string, fields ...Field) error {
	l.logger.Error(message, toZapFields(fields)...)
	return nil
}

func (l zapLogger) Sync() error {
	if err := l.logger.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) {
		return err
	}
	return nil
}

func toZapFields(fields []Field) []zap.Field {
	result := make([]zap.Field, 0, len(fields))
	for _, field := range fields {
		result = append(result, zap.Any(field.Key, field.Value))
	}
	return result
}
