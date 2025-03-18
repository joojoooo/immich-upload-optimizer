package main

import "log"

type customLogger struct {
	logger    *log.Logger
	prefix    string
	errPrefix string
}

func newCustomLogger(baseLogger interface{}, additionalPrefix string) *customLogger {
	switch logger := baseLogger.(type) {
	case *log.Logger:
		return &customLogger{
			logger: logger,
			prefix: additionalPrefix,
		}
	case *customLogger:
		return &customLogger{
			logger: logger.logger,
			prefix: logger.prefix + additionalPrefix,
		}
	default:
		panic("unsupported logger type")
	}
}

func (cl *customLogger) Println(v ...interface{}) {
	cl.logger.Println(cl.prefix, v)
}

func (cl *customLogger) Printf(format string, v ...interface{}) {
	cl.logger.Printf(cl.prefix+format, v...)
}

func (cl *customLogger) SetErrPrefix(prefix string) {
	cl.errPrefix = prefix
}

func (cl *customLogger) Error(err error, errorType string) bool {
	if err != nil {
		cl.logger.Printf(cl.prefix+cl.errPrefix+errorType+": %v", err)
		return true
	}
	return false
}
