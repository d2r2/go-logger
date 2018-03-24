package logger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"log/syslog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/d2r2/go-shell/shell"
	"github.com/davecgh/go-spew/spew"
)

type LoggerLevel int

const (
	PanicLevel LoggerLevel = iota
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
)

func (v LoggerLevel) String() string {
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

func (v *LoggerLevel) LongStr() string {
	return v.String()
}

func (v LoggerLevel) ShortStr() string {
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

type LogFile struct {
	sync.RWMutex
	Path string
	File *os.File
}

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

func (v *Logger) NewPackageLogger(packageName string, level LoggerLevel) *PackageLogger {
	v.Lock()
	defer v.Unlock()
	m := &PackageLogger{parent: v, packageName: packageName, level: level}
	v.packages = append(v.packages, m)
	return m
}

type logFile struct {
	FileInfo os.FileInfo
	Index    int
}

type sortLogFiles struct {
	Items []logFile
}

func (sf *sortLogFiles) Len() int {
	return len(sf.Items)
}

func (sf *sortLogFiles) Less(i, j int) bool {
	return sf.Items[j].Index < sf.Items[i].Index
}

func (sf *sortLogFiles) Swap(i, j int) {
	item := sf.Items[i]
	sf.Items[i] = sf.Items[j]
	sf.Items[j] = item
}

func findStringSubmatchIndexes(r *regexp.Regexp, s string) map[string][2]int {
	captures := make(map[string][2]int)
	ind := r.FindStringSubmatchIndex(s)
	names := r.SubexpNames()
	for i, name := range names {
		if name != "" && i < len(ind)/2 {
			if ind[i*2] != -1 && ind[i*2+1] != -1 {
				captures[name] = [2]int{ind[i*2], ind[i*2+1]}
			}
		}
	}
	return captures
}

func extractIndex(item os.FileInfo) int {
	r := regexp.MustCompile(`.+\.log(\.(?P<index>\d+))?`)
	fileName := path.Base(item.Name())
	m := findStringSubmatchIndexes(r, fileName)
	if v, ok := m["index"]; ok {
		i, _ := strconv.Atoi(fileName[v[0]:v[1]])
		return i
	} else {
		return 0
	}
}

func (v *LogFile) Close() error {
	v.Lock()
	defer v.Unlock()
	if v.File != nil {
		err := v.File.Close()
		v.File = nil
		return err
	}
	return nil
}

func (v *LogFile) getRotatedFileList() ([]logFile, error) {
	var list []logFile
	err := filepath.Walk(path.Dir(v.Path), func(p string,
		info os.FileInfo, err error) error {
		pattern := "*" + path.Base(v.Path) + "*"
		if ok, err := path.Match(pattern, path.Base(p)); ok && err == nil {
			i := extractIndex(info)
			list = append(list, logFile{FileInfo: info, Index: i})
		} else if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s := &sortLogFiles{Items: list}
	sort.Sort(s)
	return s.Items, nil
}

func (v *LogFile) doRotate(items []logFile, rotateMaxCount int) error {
	if len(items) > 0 {
		// delete last files
		deleteCount := len(items) - rotateMaxCount + 1
		if deleteCount > 0 {
			for i := 0; i < deleteCount; i++ {
				err := os.Remove(items[i].FileInfo.Name())
				if err != nil {
					return err
				}
			}
			items = items[deleteCount:]
		}
		// change names of rest files
		baseFilePath := items[len(items)-1].FileInfo.Name()
		movs := make([]int, len(items))
		// 1st round to change names
		for i, item := range items {
			movs[i] = i + 100000
			err := os.Rename(item.FileInfo.Name(),
				fmt.Sprintf("%s.%d", baseFilePath, movs[i]))
			if err != nil {
				return err
			}
		}
		// 2nd round to change names
		for i, item := range movs {
			err := os.Rename(fmt.Sprintf("%s.%d", baseFilePath, item),
				fmt.Sprintf("%s.%d", baseFilePath, len(items)-i))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *LogFile) rotateFiles(rotateMaxSize int64, rotateMaxCount int) error {
	fs, err := v.File.Stat()
	if err != nil {
		return err
	}
	if fs.Size() > rotateMaxSize {
		err = v.Close()
		if err != nil {
			return err
		}
		list, err := v.getRotatedFileList()
		if err != nil {
			return err
		}
		if err = v.doRotate(list, rotateMaxCount); err != nil {
			return err
		}
	}
	return nil
}

func (v *LogFile) getFile() (*os.File, error) {
	v.Lock()
	defer v.Unlock()
	if v.File == nil {
		file, err := os.OpenFile(v.Path, os.O_RDWR|os.O_APPEND, 0660)
		if err != nil {
			file, err = os.Create(v.Path)
			if err != nil {
				return nil, err
			}
		}
		v.File = file
	}
	return v.File, nil
}

func (v *LogFile) writeToFile(msg string, rotateMaxSize int64, rotateMaxCount int) error {
	file, err := v.getFile()
	if err != nil {
		return err
	}
	v.Lock()
	defer v.Unlock()
	var buf bytes.Buffer
	buf.WriteString(msg)
	buf.WriteString(fmt.Sprintln())
	if _, err := io.Copy(file, &buf); err != nil {
		return err
	}
	//	if err = file.Sync(); err != nil {
	//		return err
	//	}
	if err := v.rotateFiles(rotateMaxSize, rotateMaxCount); err != nil {
		return err
	}

	return nil
}

type PackageLogger struct {
	sync.RWMutex
	parent      *Logger
	packageName string
	level       LoggerLevel
	syslog      *syslog.Writer
}

const (
	nocolor = 0
	red     = 31
	green   = 32
	yellow  = 33
	blue    = 34
	gray    = 37
)

type IndentKind int

const (
	LeftIndent = iota
	CenterIndent
	RightIndent
)

func cutOrIndentText(text string, length int, indent IndentKind) string {
	if length < 0 {
		return text
	} else if len(text) > length {
		text = text[:length]
	} else {
		switch indent {
		case LeftIndent:
			text = text + strings.Repeat(" ", length-len(text))
		case RightIndent:
			text = strings.Repeat(" ", length-len(text)) + text
		case CenterIndent:
			text = strings.Repeat(" ", (length-len(text))/2) + text +
				strings.Repeat(" ", length-len(text)-(length-len(text))/2)

		}
	}
	return text
}

func fmtStr(colored bool, level LoggerLevel, levelFormat LevelFormat, appName string,
	packageName string, packagePrintLength int, message string, format string) string {
	var colorPfx, colorSfx string
	if colored {
		var levelColor int
		switch level {
		case DebugLevel:
			levelColor = gray
		case WarnLevel:
			levelColor = yellow
		case ErrorLevel, PanicLevel:
			levelColor = red
		default:
			levelColor = blue
		}
		colorPfx = "\x1b[" + strconv.Itoa(levelColor) + "m"
		colorSfx = "\x1b[0m"
	}
	arg1 := time.Now().Format("2006-01-02T15:04:05.000")
	arg2 := appName
	arg3 := cutOrIndentText(packageName, packagePrintLength, RightIndent)
	var lvlLen int
	var lvlStr string
	switch levelFormat {
	case LevelShort:
		lvlLen = ShortLevelLen
		lvlStr = level.ShortStr()
	case LevelLong:
		lvlLen = LongLevelLen
		lvlStr = level.LongStr()
	}
	arg4 := colorPfx + cutOrIndentText(strings.ToUpper(lvlStr), lvlLen, LeftIndent) + colorSfx
	arg5 := message
	out := fmt.Sprintf(format, arg1, arg2, arg3, arg4, arg5)
	return out
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

func (v *PackageLogger) getSyslog(level LoggerLevel, levelFormat LevelFormat,
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

func (v *PackageLogger) writeToSyslog(level LoggerLevel,
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

func (v *PackageLogger) print(level LoggerLevel, msg string) {
	if v.level >= level {
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

func (v *PackageLogger) Printf(level LoggerLevel, format string, args ...interface{}) {
	if v.level >= level {
		msg := spew.Sprintf(format, args...)
		v.print(level, msg)
	}
}

func (v *PackageLogger) Print(level LoggerLevel, args ...interface{}) {
	if v.level >= level {
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

func NewPackageLogger(module string, level LoggerLevel) *PackageLogger {
	return lgr.NewPackageLogger(module, level)
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
