package wego

import (
	"sync/atomic"

	"github.com/Jinchenyuan/wego/logger"
)

var globalMesa atomic.Value // stores *Mesa

func SetGlobalMesa(m *Mesa) { globalMesa.Store(m) }
func GetGlobalMesa() *Mesa {
	v := globalMesa.Load()
	if v == nil {
		return nil
	}
	return v.(*Mesa)
}

var globalLogger atomic.Value // stores Logger

func SetGlobalLogger(l *logger.Logger) { globalLogger.Store(l) }
func GetGlobalLogger() *logger.Logger {
	v := globalLogger.Load()
	if v == nil {
		return nil
	}
	return v.(*logger.Logger)
}
