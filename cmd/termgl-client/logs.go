package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/rivo/tview"
)

// LogLevel represents the severity of a log message
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

// LogManager manages the log panel and message history
type LogManager struct {
	// textView is the tview component for displaying logs
	textView *tview.TextView

	// messages stores recent log messages
	messages []LogMessage

	// maxMessages is the maximum number of messages to keep
	maxMessages int

	// mu protects concurrent access to messages
	mu sync.Mutex

	// autoScroll controls whether new messages auto-scroll
	autoScroll bool
}

// LogMessage represents a single log entry
type LogMessage struct {
	Time    time.Time
	Level   LogLevel
	Message string
}

// NewLogManager creates a new log manager
func NewLogManager(maxMessages int) *LogManager {
	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetMaxLines(maxMessages)

	textView.SetBorder(true).SetTitle(" Logs ")

	lm := &LogManager{
		textView:    textView,
		messages:    make([]LogMessage, 0, maxMessages),
		maxMessages: maxMessages,
		autoScroll:  true,
	}

	// Note: Scroll control can be added later if needed

	return lm
}

// GetView returns the tview component
func (lm *LogManager) GetView() tview.Primitive {
	return lm.textView
}

// AddLog adds a log message with the specified level
func (lm *LogManager) AddLog(level LogLevel, format string, args ...interface{}) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Create log message
	msg := LogMessage{
		Time:    time.Now(),
		Level:   level,
		Message: fmt.Sprintf(format, args...),
	}

	// Add to messages slice
	lm.messages = append(lm.messages, msg)

	// Trim old messages if we exceed max
	if len(lm.messages) > lm.maxMessages {
		lm.messages = lm.messages[len(lm.messages)-lm.maxMessages:]
	}

	// Update display
	lm.refresh()
}

// Debug logs a debug message
func (lm *LogManager) Debug(format string, args ...interface{}) {
	lm.AddLog(LogLevelDebug, format, args...)
}

// Info logs an info message
func (lm *LogManager) Info(format string, args ...interface{}) {
	lm.AddLog(LogLevelInfo, format, args...)
}

// Warn logs a warning message
func (lm *LogManager) Warn(format string, args ...interface{}) {
	lm.AddLog(LogLevelWarn, format, args...)
}

// Error logs an error message
func (lm *LogManager) Error(format string, args ...interface{}) {
	lm.AddLog(LogLevelError, format, args...)
}

// refresh updates the text view with current messages
func (lm *LogManager) refresh() {
	// Build formatted output
	lm.textView.Clear()

	for _, msg := range lm.messages {
		// Color code based on level
		color := lm.getColorForLevel(msg.Level)
		levelStr := fmt.Sprintf("[%s]%-5s[-]", color, msg.Level)
		timeStr := msg.Time.Format("15:04:05")

		// Format: [HH:MM:SS] LEVEL Message
		line := fmt.Sprintf("[gray]%s[-] %s %s\n", timeStr, levelStr, msg.Message)
		fmt.Fprint(lm.textView, line)
	}

	// Auto-scroll to bottom if enabled
	if lm.autoScroll {
		lm.textView.ScrollToEnd()
	}
}

// getColorForLevel returns the tview color tag for a log level
func (lm *LogManager) getColorForLevel(level LogLevel) string {
	switch level {
	case LogLevelDebug:
		return "gray"
	case LogLevelInfo:
		return "white"
	case LogLevelWarn:
		return "yellow"
	case LogLevelError:
		return "red"
	default:
		return "white"
	}
}

// Clear removes all log messages
func (lm *LogManager) Clear() {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.messages = make([]LogMessage, 0, lm.maxMessages)
	lm.textView.Clear()
}

// SetAutoScroll enables or disables automatic scrolling
func (lm *LogManager) SetAutoScroll(enabled bool) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.autoScroll = enabled
}
