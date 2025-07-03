package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

type Logger struct {
	serviceName string
	logger      *log.Logger
	file        *os.File
}

func NewLogger(serviceName string) *Logger {
	return &Logger{
		serviceName: serviceName,
		logger:      log.New(os.Stdout, "", 0),
	}
}

// NewFileLogger creates a logger that writes to both console and file
func NewFileLogger(serviceName, logDir string) (*Logger, error) {
	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create log file with timestamp (date and time)
	timestamp := time.Now().Format("2006-01-02_15-04")
	logFile := filepath.Join(logDir, fmt.Sprintf("%s_%s.log", serviceName, timestamp))
	
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Create multi-writer for both console and file
	multiWriter := io.MultiWriter(os.Stdout, file)

	return &Logger{
		serviceName: serviceName,
		logger:      log.New(multiWriter, "", 0),
		file:        file,
	}, nil
}

func (l *Logger) formatMessage(level, message string, args ...interface{}) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	formattedMessage := fmt.Sprintf(message, args...)
	return fmt.Sprintf("[%s] %s [%s] - %s", level, timestamp, l.serviceName, formattedMessage)
}

func (l *Logger) Info(message string, args ...interface{}) {
	l.logger.Print(l.formatMessage("INFO", message, args...))
}

func (l *Logger) Error(message string, args ...interface{}) {
	l.logger.Print(l.formatMessage("ERROR", message, args...))
}

func (l *Logger) Debug(message string, args ...interface{}) {
	l.logger.Print(l.formatMessage("DEBUG", message, args...))
}

func (l *Logger) Warning(message string, args ...interface{}) {
	l.logger.Print(l.formatMessage("WARNING", message, args...))
}

func (l *Logger) SetOutput(w io.Writer) {
	l.logger.SetOutput(w)
}

func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Package-level functions for convenience
var defaultLogger = NewLogger("")

func Info(message string, args ...interface{}) {
	defaultLogger.Info(message, args...)
}

func Error(message string, args ...interface{}) {
	defaultLogger.Error(message, args...)
}

func Warn(message string, args ...interface{}) {
	defaultLogger.Warning(message, args...)
}

func Debug(message string, args ...interface{}) {
	defaultLogger.Debug(message, args...)
} 