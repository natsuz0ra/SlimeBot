package logging

import (
	"time"
)

func Span(name string, start time.Time) {
	Info("perf_span", "name", name, "ms", time.Since(start).Milliseconds())
}
