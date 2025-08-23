package utils

import (
	"log"
	"os"
)

var (
	infoLogger    *log.Logger
	warningLogger *log.Logger
	errorLogger   *log.Logger
)

func InitLogger() {
	infoLogger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	warningLogger = log.New(os.Stdout, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile)
	errorLogger = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
}

func LogInfo(message string) {
	infoLogger.Println(message)
}

func LogWarning(message string) {
	warningLogger.Println(message)
}

func LogError(message string, err error) {
	if err != nil {
		errorLogger.Printf("%s: %v", message, err)
	} else {
		errorLogger.Println(message)
	}
}

func LogFatal(message string, err error) {
	if err != nil {
		errorLogger.Fatalf("%s: %v", message, err)
	} else {
		errorLogger.Fatal(message)
	}
}
