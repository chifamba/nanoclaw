package logger

import (
	"fmt"
	"log"
	"os"
)

var (
	infoLogger  = log.New(os.Stderr, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	warnLogger  = log.New(os.Stderr, "WARN: ", log.Ldate|log.Ltime|log.Lshortfile)
	errorLogger = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
	debugLogger = log.New(os.Stderr, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)
)

func Info(msg string, args ...interface{}) {
	infoLogger.Output(2, fmt.Sprintf("%s %v", msg, args))
}

func Warn(msg string, args ...interface{}) {
	warnLogger.Output(2, fmt.Sprintf("%s %v", msg, args))
}

func Error(msg string, args ...interface{}) {
	errorLogger.Output(2, fmt.Sprintf("%s %v", msg, args))
}

func Debug(msg string, args ...interface{}) {
	if os.Getenv("LOG_LEVEL") == "debug" {
		debugLogger.Output(2, fmt.Sprintf("%s %v", msg, args))
	}
}
