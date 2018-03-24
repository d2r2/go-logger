package logger

import (
	"sync"

	"github.com/Sirupsen/logrus"
)

var (
	lgr1234 *Logger
	lock    sync.Mutex
)

// Won't compile if specific interfaces
// can't be realized by coresponding structs.
var (
	_ logrus.Hook = new(consoleHook)
	_ logrus.Hook = new(fileLogHook)
)

func GetModuleLogger(module string, level logrus.Level) *logrus.Entry {
	lg := getLogger()
	lg.SetModuleLoggerLevel(module, level)
	entry := lg.GetModuleLogger(module)
	return entry
}

func SetLogFilePath(logFilePath string) {
	lg := getLogger()
	lg.SetLogFilePath(logFilePath)
}

func SetModuleLength(modLength int) {
	lg := getLogger()
	lg.SetModuleLength(modLength)
}

func SetRotateParams(maxSize int64, maxCount int) {
	lg := getLogger()
	lg.SetRotateParams(maxSize, maxCount)
}
