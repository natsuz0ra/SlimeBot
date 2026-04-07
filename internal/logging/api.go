package logging

import (
	"fmt"

	"go.uber.org/zap"
)

func Fields(kv ...any) []zap.Field {
	if len(kv) == 0 {
		return nil
	}
	fields := make([]zap.Field, 0, (len(kv)+1)/2)
	for i := 0; i < len(kv); i += 2 {
		key := fmt.Sprintf("arg_%d", i)
		if s, ok := kv[i].(string); ok && s != "" {
			key = s
		}
		if i+1 >= len(kv) {
			fields = append(fields, zap.Any(key, nil))
			break
		}
		if err, ok := kv[i+1].(error); ok {
			fields = append(fields, zap.NamedError(key, err))
			continue
		}
		fields = append(fields, zap.Any(key, kv[i+1]))
	}
	return fields
}

func Info(msg string, kv ...any) {
	zap.L().Info(msg, Fields(kv...)...)
}

func Warn(msg string, kv ...any) {
	zap.L().Warn(msg, Fields(kv...)...)
}

func Error(msg string, kv ...any) {
	zap.L().Error(msg, Fields(kv...)...)
}
