package logger

import (
	"context"
	"fmt"
	"log"
	"log/syslog"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/d2r2/go-shell/shell"
	"github.com/davecgh/go-spew/spew"
)

type LogLevel int

const (
	PanicLevel LogLevel = iota
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
)

func (v LogLevel) String() string {
	switch v {
	case PanicLevel:
		return "Fatal"
	case ErrorLevel:
		return "Error"
	case WarnLevel:
		return "Warning"
	case InfoLevel:
		return "Information"
	case DebugLevel:
		return "Debug"
	default:
		return "<undefined>"
	}
}

func (v *LogLevel) LongStr() string {
	return v.String()
}

func (v LogLevel) ShortStr() string {
	switch v {
	case PanicLevel:
		return "Pamic"
	case ErrorLevel:
		return "Error"
	case WarnLevel:
		return "Warn"
	case InfoLevel:
		return "Info"
	case DebugLevel:
		return "Debug"
	default:
		return "undef"
	}
}

type LevelFormat int

const (
	LevelShort LevelFormat = iota
	LevelLong
)

const (
	ShortLevelLen = 5
	LongLevelLen  = 11
)

type Logger struct {
	sync.RWMutex
	log                *log.Logger
	packages           []*PackageLogger
	packagePrintLength int
	levelFormat        LevelFormat
	logFile            *LogFile
	rotateMaxSize      int64
	rotateMaxCount     int
	appName            string
	enableSyslog       bool
}

func NewLogger() *Logger {
	log := log.New(os.Stdout, "", 0)
	l := &Logger{
		log:                log,
		levelFormat:        LevelShort,
		packagePrintLength: 8,
		rotateMaxSize:      1024 * 1024 * 512,
		rotateMaxCount:     3,
	}
	return l
}

func (v *Logger) Close() error {
	v.Lock()
	defer v.Unlock()

	for _, pack := range v.packages {
		pack.Close()
	}
	v.packages = nil

	if v.logFile != nil {
		v.logFile.Close()
	}
	return nil
}

func (v *Logger) SetRotateParams(rotateMaxSize int64, rotateMaxCount int) {
	v.Lock()
	defer v.Unlock()
	v.rotateMaxSize = rotateMaxSize
	v.rotateMaxCount = rotateMaxCount
}

func (v *Logger) GetRotateMaxSize() int64 {
	v.Lock()
	defer v.Unlock()
	return v.rotateMaxSize
}

func (v *Logger) GetRotateMaxCount() int {
	v.Lock()
	defer v.Unlock()
	return v.rotateMaxCount
}

func (v *Logger) SetLevelFormat(levelFormat LevelFormat) {
	v.Lock()
	defer v.Unlock()
	v.levelFormat = levelFormat
}

func (v *Logger) GetLevelFormat() LevelFormat {
	v.RLock()
	defer v.RUnlock()
	return v.levelFormat
}

func (v *Logger) SetApplicationName(appName string) {
	v.Lock()
	defer v.Unlock()
	v.appName = appName
}

func (v *Logger) GetApplicationName() string {
	v.RLock()
	defer v.RUnlock()
	return v.appName
}

func (v *Logger) EnableSyslog(enable bool) {
	v.Lock()
	defer v.Unlock()
	v.enableSyslog = enable
}

func (v *Logger) GetSyslogEnabled() bool {
	v.RLock()
	defer v.RUnlock()
	return v.enableSyslog
}

func (v *Logger) SetPackagePrintLength(packagePrintLength int) {
	v.Lock()
	defer v.Unlock()
	v.packagePrintLength = packagePrintLength
}

func (v *Logger) GetPackagePrintLength() int {
	v.RLock()
	defer v.RUnlock()
	return v.packagePrintLength
}

func (v *Logger) SetLogFileName(logFilePath string) error {
	if path.Ext(logFilePath) == "" {
		logFilePath += ".log"
	}
	fp, err := filepath.Abs(logFilePath)
	if err != nil {
		return err
	}
	v.Lock()
	defer v.Unlock()
	lf := &LogFile{Path: fp}
	v.logFile = lf
	return nil
}

func (v *Logger) GetLogFileInfo() *LogFile {
	v.Lock()
	defer v.Unlock()
	return v.logFile
}

func (v *Logger) NewPackageLogger(packageName string, level LogLevel) *PackageLogger {
	v.Lock()
	defer v.Unlock()
	p := &PackageLogger{parent: v, packageName: packageName, level: level}
	v.packages = append(v.packages, p)
	return p
}

func (v *Logger) ChangePackageLogLevel(packageName string, level LogLevel) error {
	var p *PackageLogger
	for _, item := range v.packages {
		if item.packageName == packageName {
			p = item
			break
		}
	}
	if p != nil {
		p.SetLogLevel(level)
	} else {
		err := fmt.Errorf("Package log %q is not found", packageName)
		return err
	}
	return nil
}

type PackageLogger struct {
	sync.RWMutex
	parent      *Logger
	packageName string
	level       LogLevel
	syslog      *syslog.Writer
}

func (v *PackageLogger) Close() error {
	v.Lock()
	defer v.Unlock()
	if v.syslog != nil {
		err := v.syslog.Close()
		v.syslog = nil
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *PackageLogger) SetLogLevel(level LogLevel) {
	v.Lock()
	defer v.Unlock()
	v.level = level
}

func (v *PackageLogger) GetLogLevel() LogLevel {
	v.RLock()
	defer v.RUnlock()
	return v.level
}

func (v *PackageLogger) getSyslog(level LogLevel, levelFormat LevelFormat,
	appName string) (*syslog.Writer, error) {
	v.Lock()
	defer v.Unlock()
	if v.syslog == nil {
		tag := fmtStr(false, level, levelFormat, appName,
			v.packageName, -1, "", "%[2]s-%[3]s")
		sl, err := syslog.New(syslog.LOG_DEBUG, tag)
		if err != nil {
			err = spew.Errorf("Failed to connect to syslog: %v\n", err)
			return nil, err
		}
		v.syslog = sl
	}
	return v.syslog, nil
}

func (v *PackageLogger) writeToSyslog(level LogLevel,
	levelFormat LevelFormat, appName string, msg string) error {
	sl, err := v.getSyslog(level, levelFormat, appName)
	if err != nil {
		return err
	}
	switch level {
	case DebugLevel:
		return sl.Debug(msg)
	case InfoLevel:
		return sl.Info(msg)
	case WarnLevel:
		return sl.Warning(msg)
	case ErrorLevel:
		return sl.Err(msg)
	case PanicLevel:
		return sl.Crit(msg)
	default:
		return sl.Debug(msg)
	}
}

func (v *PackageLogger) print(level LogLevel, msg string) {
	lvl := v.GetLogLevel()
	if lvl >= level {
		levelFormat := v.parent.GetLevelFormat()
		packagePrintLen := v.parent.GetPackagePrintLength()
		appName := v.parent.GetApplicationName()
		if appName == "" {
			appName = os.Args[0]
		}
		out1 := fmtStr(true, level, levelFormat, appName,
			v.packageName, packagePrintLen, msg, "%[1]s [%[3]s] %[4]s  %[5]s")
		// File output
		if lf := v.parent.GetLogFileInfo(); lf != nil {
			rotateMaxSize := v.parent.GetRotateMaxSize()
			rotateMaxCount := v.parent.GetRotateMaxCount()
			out2 := fmtStr(false, level, levelFormat, appName,
				v.packageName, packagePrintLen, msg, "%[1]s [%[3]s] %[4]s  %[5]s")
			if err := lf.writeToFile(out2, rotateMaxSize, rotateMaxCount); err != nil {
				err = spew.Errorf("Failed to report syslog message %q: %v\n", out2, err)
				v.parent.log.Fatal(err)
			}
		}
		// Syslog output
		if v.parent.GetSyslogEnabled() {
			if err := v.writeToSyslog(level, levelFormat, appName, msg); err != nil {
				err = spew.Errorf("Failed to report syslog message %q: %v\n", msg, err)
				v.parent.log.Fatal(err)
			}
		}
		// Console output
		v.parent.log.Print(out1 + fmt.Sprintln())
		// Check panic event
		if level == PanicLevel {
			panic(out1)
		}
	}
}

func (v *PackageLogger) Printf(level LogLevel, format string, args ...interface{}) {
	lvl := v.GetLogLevel()
	if lvl >= level {
		msg := spew.Sprintf(format, args...)
		v.print(level, msg)
	}
}

func (v *PackageLogger) Print(level LogLevel, args ...interface{}) {
	lvl := v.GetLogLevel()
	if lvl >= level {
		msg := spew.Sprint(args...)
		v.print(level, msg)
	}
}

func (v *PackageLogger) Debugf(format string, args ...interface{}) {
	v.Printf(DebugLevel, format, args...)
}

func (v *PackageLogger) Debug(args ...interface{}) {
	v.Print(DebugLevel, args...)
}

func (v *PackageLogger) Infof(format string, args ...interface{}) {
	v.Printf(InfoLevel, format, args...)
}

func (v *PackageLogger) Info(args ...interface{}) {
	v.Print(InfoLevel, args...)
}

func (v *PackageLogger) Warnf(format string, args ...interface{}) {
	v.Printf(WarnLevel, format, args...)
}

func (v *PackageLogger) Warn(args ...interface{}) {
	v.Print(WarnLevel, args...)
}

func (v *PackageLogger) Errorf(format string, args ...interface{}) {
	v.Printf(ErrorLevel, format, args...)
}

func (v *PackageLogger) Error(args ...interface{}) {
	v.Print(ErrorLevel, args...)
}

func (v *PackageLogger) Panicf(format string, args ...interface{}) {
	v.Printf(PanicLevel, format, args...)
}

func (v *PackageLogger) Panic(args ...interface{}) {
	v.Print(PanicLevel, args...)
}

var (
	lgr *Logger
)

func SetLevelFormat(levelFormat LevelFormat) {
	lgr.SetLevelFormat(levelFormat)
}

func SetPackagePrintLength(packagePrintLength int) {
	lgr.SetPackagePrintLength(packagePrintLength)
}

func SetRotateParams(rotateMaxSize int64, rotateMaxCount int) {
	lgr.SetRotateParams(rotateMaxSize, rotateMaxCount)
}

func NewPackageLogger(module string, level LogLevel) *PackageLogger {
	return lgr.NewPackageLogger(module, level)
}

func ChangePackageLogLevel(packageName string, level LogLevel) error {
	return lgr.ChangePackageLogLevel(packageName, level)
}

func SetLogFileName(logFilePath string) error {
	return lgr.SetLogFileName(logFilePath)
}

func SetApplicationName(appName string) {
	lgr.SetApplicationName(appName)
}

func EnableSyslog(enable bool) {
	lgr.EnableSyslog(enable)
}

func FinalizeLogger() error {
	var err error
	if lgr != nil {
		err = lgr.Close()
	}
	lgr.Lock()
	defer lgr.Unlock()
	lgr = nil
	return err
}

func init() {
	lgr = NewLogger()
	ctx, cancel := context.WithCancel(context.Background())
	shell.CloseContextOnKillSignal(cancel)

	go func(logger *Logger) {
		<-ctx.Done()
		logger.Close()
		lgr.Lock()
		defer lgr.Unlock()
		lgr = nil
		log.Println("Finalizing logger")
	}(lgr)
}
