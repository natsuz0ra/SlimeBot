package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"slimebot/internal/runtime"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Mode string

const (
	ModeCLI    Mode = "cli"
	ModeServer Mode = "server"
)

type Options struct {
	Mode          Mode
	LogDir        string
	RetentionDays int
	MaxFiles      int
	Now           func() time.Time
	Level         zapcore.Level
	ConsoleWriter io.Writer
	EnableFile    bool
}

func Init(opts Options) (*zap.Logger, func(), error) {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()
	if opts.Mode == "" {
		opts.Mode = ModeServer
	}
	if opts.RetentionDays <= 0 {
		opts.RetentionDays = 14
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 20
	}
	if opts.Level == 0 {
		opts.Level = zapcore.InfoLevel
	}
	if strings.TrimSpace(opts.LogDir) == "" {
		opts.LogDir = filepath.Join(runtime.SlimeBotHomeDir(), "log")
	}
	if !opts.EnableFile {
		opts.EnableFile = true
	}

	if err := os.MkdirAll(opts.LogDir, os.ModePerm); err != nil {
		logger := zap.NewNop()
		zap.ReplaceGlobals(logger)
		return logger, func() {}, nil
	}
	_ = cleanupOldLogs(opts.LogDir, now, opts.RetentionDays, opts.MaxFiles)

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "time"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.LowercaseLevelEncoder

	cores := make([]zapcore.Core, 0, 2)
	closeFns := make([]func(), 0, 1)

	if opts.EnableFile {
		logFilePath := filepath.Join(opts.LogDir, fmt.Sprintf("%s-%s.log", opts.Mode, now.Format("20060102")))
		file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			cores = append(cores, zapcore.NewCore(zapcore.NewJSONEncoder(encoderCfg), zapcore.AddSync(file), opts.Level))
			closeFns = append(closeFns, func() { _ = file.Close() })
		}
	}

	if opts.Mode == ModeServer {
		consoleWriter := opts.ConsoleWriter
		if consoleWriter == nil {
			consoleWriter = os.Stderr
		}
		consoleCfg := zap.NewDevelopmentEncoderConfig()
		consoleCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		cores = append(cores, zapcore.NewCore(zapcore.NewConsoleEncoder(consoleCfg), zapcore.AddSync(consoleWriter), opts.Level))
	}

	if len(cores) == 0 {
		logger := zap.NewNop()
		zap.ReplaceGlobals(logger)
		return logger, func() {}, nil
	}

	logger := zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.AddCallerSkip(1))
	zap.ReplaceGlobals(logger)

	cleanup := func() {
		_ = logger.Sync()
		for _, fn := range closeFns {
			fn()
		}
	}
	return logger, cleanup, nil
}

func cleanupOldLogs(logDir string, now time.Time, retentionDays int, maxFiles int) error {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return err
	}
	type item struct {
		path    string
		modTime time.Time
	}
	logs := make([]item, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
			continue
		}
		fullPath := filepath.Join(logDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		logs = append(logs, item{path: fullPath, modTime: info.ModTime()})
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].modTime.After(logs[j].modTime) })

	cutoff := now.AddDate(0, 0, -retentionDays)
	for idx, entry := range logs {
		if !entry.modTime.After(cutoff) || idx >= maxFiles {
			_ = os.Remove(entry.path)
		}
	}
	return nil
}
