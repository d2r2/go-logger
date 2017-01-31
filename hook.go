package logger

/*
type message struct {
	Entry     logrus.Entry
	Terminate bool
}

type threadSafeHook struct {
	logger *Logger
}

func newThreadSafeHook(logger *Logger) *threadSafeHook {
	hook := &threadSafeHook{
		logger: logger,
	}
	return hook
}

func (hook *threadSafeHook) Fire(entry *logrus.Entry) error {
	return hook.logger.fire(entry)
}

func (hook *threadSafeHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
	}
}
*/
