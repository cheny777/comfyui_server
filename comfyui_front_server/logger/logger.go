package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var (
	levelNames = map[LogLevel]string{
		DEBUG: "DEBUG",
		INFO:  "INFO",
		WARN:  "WARN",
		ERROR: "ERROR",
	}

	currentLevel LogLevel = INFO
	mu           sync.RWMutex
	logger       *log.Logger
)

func init() {
	logger = log.New(os.Stdout, "", 0)
}

// SetLevel 设置日志级别
func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = level
}

// SetLevelString 通过字符串设置日志级别
func SetLevelString(levelStr string) {
	levelStr = strings.ToUpper(levelStr)
	switch levelStr {
	case "DEBUG":
		SetLevel(DEBUG)
	case "INFO":
		SetLevel(INFO)
	case "WARN":
		SetLevel(WARN)
	case "ERROR":
		SetLevel(ERROR)
	default:
		SetLevel(INFO)
	}
}

// GetLevel 获取当前日志级别
func GetLevel() LogLevel {
	mu.RLock()
	defer mu.RUnlock()
	return currentLevel
}

// shouldLog 检查是否应该记录该级别的日志
func shouldLog(level LogLevel) bool {
	mu.RLock()
	defer mu.RUnlock()
	return level >= currentLevel
}

// getCallerInfo 获取调用者信息（文件路径和行号）
func getCallerInfo(skip int) (file string, line int) {
	_, filePath, lineNum, ok := runtime.Caller(skip)
	if !ok {
		return "unknown", 0
	}
	
	// 如果路径包含构建时的路径（通常是绝对路径或包含用户目录），只显示文件名
	// 使用 -trimpath 构建后，路径会更简洁，但仍可能包含模块路径
	// 优先显示相对于模块的路径，如果无法确定则只显示文件名
	if filepath.IsAbs(filePath) {
		// 尝试提取模块路径后的部分
		// 例如：/tmp/go-build123/comfyui_front_server/main.go -> main.go
		// 或者：comfyui_front_server/main.go -> main.go
		base := filepath.Base(filePath)
		dir := filepath.Dir(filePath)
		
		// 如果目录名看起来像临时构建目录，只返回文件名
		if strings.Contains(dir, "go-build") || strings.Contains(dir, "/tmp/") {
			return base, lineNum
		}
		
		// 尝试提取模块名后的路径
		parts := strings.Split(filePath, string(filepath.Separator))
		for i, part := range parts {
			if part == "comfyui_front_server" && i+1 < len(parts) {
				// 找到模块名，返回模块名后的路径
				return strings.Join(parts[i:], string(filepath.Separator)), lineNum
			}
		}
		
		// 如果找不到模块名，返回文件名
		return base, lineNum
	}
	
	// 相对路径，直接返回
	return filePath, lineNum
}

// formatMessage 格式化日志消息
func formatMessage(level LogLevel, format string, args ...interface{}) string {
	levelName := levelNames[level]
	file, line := getCallerInfo(3) // skip: formatMessage -> Debug/Info/Warn/Error -> 实际调用
	
	// 格式化消息内容
	var message string
	if format == "" {
		if len(args) > 0 {
			message = fmt.Sprint(args...)
		}
	} else {
		message = fmt.Sprintf(format, args...)
	}
	
	// 格式: [级别] 文件路径:行号 消息
	return fmt.Sprintf("[%s] %s:%d %s", levelName, file, line, message)
}

// Debug 打印DEBUG级别日志
func Debug(args ...interface{}) {
	if !shouldLog(DEBUG) {
		return
	}
	logger.Println(formatMessage(DEBUG, "", args...))
}

// Debugf 打印DEBUG级别日志（格式化）
func Debugf(format string, args ...interface{}) {
	if !shouldLog(DEBUG) {
		return
	}
	logger.Println(formatMessage(DEBUG, format, args...))
}

// Info 打印INFO级别日志
func Info(args ...interface{}) {
	if !shouldLog(INFO) {
		return
	}
	logger.Println(formatMessage(INFO, "", args...))
}

// Infof 打印INFO级别日志（格式化）
func Infof(format string, args ...interface{}) {
	if !shouldLog(INFO) {
		return
	}
	logger.Println(formatMessage(INFO, format, args...))
}

// Warn 打印WARN级别日志
func Warn(args ...interface{}) {
	if !shouldLog(WARN) {
		return
	}
	logger.Println(formatMessage(WARN, "", args...))
}

// Warnf 打印WARN级别日志（格式化）
func Warnf(format string, args ...interface{}) {
	if !shouldLog(WARN) {
		return
	}
	logger.Println(formatMessage(WARN, format, args...))
}

// Error 打印ERROR级别日志
func Error(args ...interface{}) {
	if !shouldLog(ERROR) {
		return
	}
	logger.Println(formatMessage(ERROR, "", args...))
}

// Errorf 打印ERROR级别日志（格式化）
func Errorf(format string, args ...interface{}) {
	if !shouldLog(ERROR) {
		return
	}
	logger.Println(formatMessage(ERROR, format, args...))
}

// Fatal 打印ERROR级别日志并退出程序
func Fatal(args ...interface{}) {
	logger.Fatal(formatMessage(ERROR, "", args...))
}

// Fatalf 打印ERROR级别日志并退出程序（格式化）
func Fatalf(format string, args ...interface{}) {
	logger.Fatalf(formatMessage(ERROR, format, args...))
}

// Print 打印日志（兼容标准log包）
func Print(args ...interface{}) {
	Info(args...)
}

// Printf 打印日志（兼容标准log包）
func Printf(format string, args ...interface{}) {
	Infof(format, args...)
}

// Println 打印日志（兼容标准log包）
func Println(args ...interface{}) {
	Info(args...)
}

