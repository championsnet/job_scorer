package utils

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name    string
		service string
		want    string
	}{
		{
			name:    "Valid service name",
			service: "TestService",
			want:    "[TestService]",
		},
		{
			name:    "Empty service name",
			service: "",
			want:    "[INFO]", // When empty, no service prefix is added
		},
		{
			name:    "Service with spaces",
			service: "Test Service",
			want:    "[Test Service]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(tt.service)
			
			// Capture log output
			var buf bytes.Buffer
			logger.SetOutput(&buf)
			
			logger.Info("test message")
			output := buf.String()
			
			if !strings.Contains(output, tt.want) {
				t.Errorf("NewLogger() service prefix = want %v in output %v", tt.want, output)
			}
			
			if !strings.Contains(output, "test message") {
				t.Errorf("NewLogger() missing message in output %v", output)
			}
		})
	}
}

func TestLoggerLevels(t *testing.T) {
	logger := NewLogger("TestService")
	
	tests := []struct {
		name     string
		logFunc  func(string, ...interface{})
		level    string
		message  string
	}{
		{
			name:     "Info level",
			logFunc:  logger.Info,
			level:    "INFO",
			message:  "info message",
		},
		{
			name:     "Error level",
			logFunc:  logger.Error,
			level:    "ERROR",
			message:  "error message",
		},
		{
			name:     "Debug level",
			logFunc:  logger.Debug,
			level:    "DEBUG",
			message:  "debug message",
		},
		{
			name:     "Warning level",
			logFunc:  logger.Warning,
			level:    "WARNING",
			message:  "warning message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger.SetOutput(&buf)
			
			tt.logFunc(tt.message)
			output := buf.String()
			
			if !strings.Contains(output, tt.level) {
				t.Errorf("Logger.%s() level = want %v in output %v", tt.name, tt.level, output)
			}
			
			if !strings.Contains(output, tt.message) {
				t.Errorf("Logger.%s() message = want %v in output %v", tt.name, tt.message, output)
			}
			
			if !strings.Contains(output, "[TestService]") {
				t.Errorf("Logger.%s() service = want [TestService] in output %v", tt.name, output)
			}
		})
	}
}

func TestLoggerFormatting(t *testing.T) {
	logger := NewLogger("TestService")
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	
	tests := []struct {
		name     string
		logFunc  func(string, ...interface{})
		format   string
		args     []interface{}
		expected string
	}{
		{
			name:     "String formatting",
			logFunc:  logger.Info,
			format:   "User %s logged in",
			args:     []interface{}{"john"},
			expected: "User john logged in",
		},
		{
			name:     "Number formatting",
			logFunc:  logger.Info,
			format:   "Found %d jobs",
			args:     []interface{}{42},
			expected: "Found 42 jobs",
		},
		{
			name:     "Multiple args",
			logFunc:  logger.Info,
			format:   "Processing %s with %d items",
			args:     []interface{}{"jobs", 5},
			expected: "Processing jobs with 5 items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc(tt.format, tt.args...)
			output := buf.String()
			
			if !strings.Contains(output, tt.expected) {
				t.Errorf("Logger formatting = want %v in output %v", tt.expected, output)
			}
		})
	}
}

func TestLoggerSetOutput(t *testing.T) {
	logger := NewLogger("TestService")
	
	// Test with different writers
	var buf1, buf2 bytes.Buffer
	
	// Set first output
	logger.SetOutput(&buf1)
	logger.Info("message1")
	
	if buf1.Len() == 0 {
		t.Error("Logger.SetOutput() first writer should receive output")
	}
	if buf2.Len() != 0 {
		t.Error("Logger.SetOutput() second writer should not receive output")
	}
	
	// Set second output
	logger.SetOutput(&buf2)
	logger.Info("message2")
	
	if !strings.Contains(buf2.String(), "message2") {
		t.Error("Logger.SetOutput() second writer should receive new output")
	}
	
	// First buffer should not receive new message
	if strings.Contains(buf1.String(), "message2") {
		t.Error("Logger.SetOutput() first writer should not receive new output")
	}
}

func TestLoggerConcurrency(t *testing.T) {
	logger := NewLogger("TestService")
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	
	// Test concurrent logging
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func(n int) {
			logger.Info("Message from goroutine %d", n)
			done <- true
		}(i)
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	
	output := buf.String()
	
	// Check that we got some output (exact count may vary due to race conditions)
	if len(output) == 0 {
		t.Error("Logger should handle concurrent writes")
	}
	
	// Check that service name appears in output
	if !strings.Contains(output, "[TestService]") {
		t.Error("Logger should maintain service name in concurrent writes")
	}
}

func TestLoggerNilOutput(t *testing.T) {
	logger := NewLogger("TestService")
	
	// This should not panic
	logger.SetOutput(io.Discard)
	logger.Info("test message")
	
	// Reset to default behavior
	logger.SetOutput(nil)
	
	// This should also not panic and should output to default logger
	logger.Info("another test message")
} 