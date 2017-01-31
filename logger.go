package logger

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sync"

	"github.com/Sirupsen/logrus"
)

const (
	PanicLevel = logrus.PanicLevel
	FatalLevel = logrus.FatalLevel
	ErrorLevel = logrus.ErrorLevel
	WarnLevel  = logrus.WarnLevel
	InfoLevel  = logrus.InfoLevel
	DebugLevel = logrus.DebugLevel
)

type Logger struct {
	loggers         map[string]*logrus.Entry
	moduleLength    int
	logFilePathFile *string
	rotateMaxSize   int64
	rotateMaxCount  int
	isTerminal      bool
	console         *consoleHook
	fileLog         *fileLogHook
	sync.Mutex
}

// Watch for OS SIGINT or SIGKILL signals. Once it happens,
// close kill channel to broadcast message about termination
// across application.
func closeChannelOnSignal(kill chan struct{}) {
	// Set up channel on which to send signal notifications
	// We must use a buffered channel or risk missing the signal
	// if we're not ready to receive when the signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	// run gorutine and block until a signal is received
	go func() {
		<-c
		// send signal to threads about pending to close
		fmt.Println("Logger: signal received, close kill channel")
		close(kill)
	}()
}

func getLogger() *Logger {
	lock.Lock()
	defer lock.Unlock()
	if lgr1234 == nil {
		lgr1234 = &Logger{
			loggers:        make(map[string]*logrus.Entry),
			isTerminal:     logrus.IsTerminal(),
			moduleLength:   10,
			rotateMaxSize:  1024 * 1024 * 512,
			rotateMaxCount: 3,
		}
	}
	return lgr1234
}

func (lg *Logger) GetModuleLogger(moduleName string) *logrus.Entry {
	lg.Lock()
	defer lg.Unlock()
	if entry, ok := lg.loggers[moduleName]; !ok {
		if lg.console == nil {
			lg.console = newConsoleHook(lg)
		}
		if lg.fileLog == nil {
			lg.fileLog = newFileLogHook(lg)
		}
		logger := logrus.New()
		logger.Out = ioutil.Discard
		logger.Hooks.Add(lg.console)
		logger.Hooks.Add(lg.fileLog)
		entry = logger.WithField("module", moduleName)
		lg.loggers[moduleName] = entry
		return entry
	} else {
		return entry
	}
}

func (lg *Logger) SetModuleLoggerLevel(moduleName string, level logrus.Level) {
	entry := lg.GetModuleLogger(moduleName)
	lg.Lock()
	defer lg.Unlock()
	entry.Logger.Level = level
}

func (lg *Logger) GetLogFilePath() *string {
	lg.Lock()
	defer lg.Unlock()
	return lg.logFilePathFile
}

func (lg *Logger) SetLogFilePath(logFilePath string) error {
	if path.Ext(logFilePath) == "" {
		logFilePath += ".log"
	}
	fp, err := filepath.Abs(logFilePath)
	if err != nil {
		return err
	}
	lg.Lock()
	defer lg.Unlock()
	lg.logFilePathFile = &fp
	return nil
}

func (lg *Logger) GetModuleLength() int {
	lg.Lock()
	defer lg.Unlock()
	return lg.moduleLength
}

func (lg *Logger) SetModuleLength(modLength int) {
	lg.Lock()
	defer lg.Unlock()
	lg.moduleLength = modLength
}

func (lg *Logger) GetRotateMaxSize() int64 {
	lg.Lock()
	defer lg.Unlock()
	return lg.rotateMaxSize
}

func (lg *Logger) GetRotateMaxCount() int {
	lg.Lock()
	defer lg.Unlock()
	return lg.rotateMaxCount
}

func (lg *Logger) SetRotateParams(maxSize int64, maxCount int) {
	lg.Lock()
	defer lg.Unlock()
	lg.rotateMaxSize = maxSize
	lg.rotateMaxCount = maxCount
}
