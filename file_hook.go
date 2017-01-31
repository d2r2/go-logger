package logger

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"

	"github.com/Sirupsen/logrus"
)

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

type fileLogHook struct {
	logger *Logger
	size   int64
	sync.Mutex
}

func newFileLogHook(logger *Logger) *fileLogHook {
	hook := &fileLogHook{logger: logger, size: -1}
	return hook
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

func (hook *fileLogHook) getSize() int64 {
	hook.Lock()
	defer hook.Unlock()
	return hook.size
}

var (
	ErrorFilePathDoesNotSetup = fmt.Errorf("File path does not set")
)

func (hook *fileLogHook) getFilePath() (string, error) {
	logPath := hook.logger.GetLogFilePath()
	if logPath != nil {
		fp := *logPath
		if hook.getSize() < 0 {
			hook.Lock()
			defer hook.Unlock()
			fi, err := os.Stat(fp)
			if os.IsNotExist(err) {
				hook.size = 0
			} else if err == nil {
				hook.size = fi.Size()
			} else if err != nil {
				return "", err
			}
		}
		return fp, nil
	} else {
		return "", ErrorFilePathDoesNotSetup
	}
}

func (hook *fileLogHook) getRotatedFileList() ([]logFile, error) {
	var list []logFile
	if fileName, err := hook.getFilePath(); err == nil {
		hook.Lock()
		defer hook.Unlock()
		err = filepath.Walk(path.Dir(fileName), func(p string,
			info os.FileInfo, err error) error {
			pattern := "*" + path.Base(fileName) + "*"
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
	} else if err != ErrorFilePathDoesNotSetup {
		return nil, err
	}
	s := &sortLogFiles{Items: list}
	sort.Sort(s)
	return s.Items, nil
}

func (hook *fileLogHook) rotateFiles() error {
	if _, err := hook.getFilePath(); err == nil {
		if hook.getSize() > hook.logger.GetRotateMaxSize() {
			list, err := hook.getRotatedFileList()
			if err != nil {
				return err
			}
			err = hook.doRotate(list)
			if err != nil {
				return err
			}
		}
	} else if err != ErrorFilePathDoesNotSetup {
		return err
	}
	return nil
}

func (hook *fileLogHook) doRotate(items []logFile) error {
	if len(items) > 0 {
		hook.Lock()
		defer hook.Unlock()
		hook.size = -1
		// delete last files
		deleteCount := len(items) - hook.logger.GetRotateMaxCount() + 1
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

func (hook *fileLogHook) writeToLog(filePath string, buf *bytes.Buffer) error {
	hook.Lock()
	defer hook.Unlock()
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_APPEND, 0660)
	if err != nil {
		file, err = os.Create(filePath)
		if err != nil {
			return err
		}
		hook.size = 0
	}
	defer file.Close()
	n, err := io.Copy(file, buf)
	if err != nil {
		hook.size = -1
		return err
	}
	if hook.size >= 0 {
		hook.size += n
	}
	return nil
}

func (hook *fileLogHook) Fire(entry *logrus.Entry) error {
	if fileName, err := hook.getFilePath(); err == nil {
		err := hook.rotateFiles()
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		isColored := false
		printEntry(&buf, entry, isColored, hook.logger.GetModuleLength())
		err = hook.writeToLog(fileName, &buf)
		if err != nil {
			return fmt.Errorf("Failed to write to log, %v\n", err)
		}
	} else if err != ErrorFilePathDoesNotSetup {
		return err
	}
	return nil
}

func (hook *fileLogHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
	}
}
