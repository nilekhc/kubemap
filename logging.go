package kubemap

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func (m *Mapper) debug(msg string) {
	if m.log.enabled {
		m.log.logger.Debug(msg)
	}
}

func (m *Mapper) info(msg string) {
	if m.log.enabled {
		m.log.logger.Info(msg)
	}
}

func (m *Mapper) warn(msg string) {
	if m.log.enabled {
		m.log.logger.Warn(msg)
	}
}

func (m *Mapper) error(msg string) {
	if m.log.enabled {
		m.log.logger.Error(msg)
	}
}

func getZapLogger(logLevel string) (*zap.SugaredLogger, error) {
	//zap config
	zapConfig := zap.NewDevelopmentConfig()

	if logLevel != "" {
		switch strings.ToLower(logLevel) {
		case "info":
			zapConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
		case "debug":
			zapConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		case "warn":
			zapConfig.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
		case "error":
			zapConfig.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
		default:
			return nil, fmt.Errorf("Cannot instantiate Mapper. Invalid Log level %s provided. Accepted values are 'info' & 'debug'", logLevel)
		}
	}

	for i := range zapConfig.OutputPaths {
		zapConfig.OutputPaths[i] = "stdout"
	}

	for i := range zapConfig.ErrorOutputPaths {
		zapConfig.ErrorOutputPaths[i] = "stdout"
	}

	logger, _ := zapConfig.Build()
	defer logger.Sync() // flushes buffer, if any
	return logger.Sugar(), nil
}
