package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	acpagent "slimebot/internal/acp/agent"
	"slimebot/internal/config"
	"slimebot/internal/constants"
	"slimebot/internal/logging"
	"slimebot/internal/runtime"
	buildversion "slimebot/internal/version"
)

func RunACP(args []string, input io.Reader, protocolOutput io.Writer) error {
	flags := flag.NewFlagSet("slimebot acp", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	model := flags.String("model", "", "SlimeBot model ID or name")
	approval := flags.String("approval-mode", constants.ApprovalModeStandard, "standard, auto_review, or auto")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || !acpagent.ValidApprovalMode(*approval) {
		return fmt.Errorf("usage: slimebot acp [--model <id-or-name>] [--approval-mode <standard|auto_review|auto>]")
	}
	_, cleanupLogs, _ := logging.Init(logging.Options{Mode: logging.ModeServer, ConsoleWriter: os.Stderr})
	defer cleanupLogs()
	if err := runtime.EnsureAndLoadEnv(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	core, err := NewCore(config.Load())
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		core.Close(closeCtx)
	}()
	rc := buildRunContext(true)
	core.ChatService.SetRunContext(rc)
	configureSkillSources(core, rc.IsCLI, rc.WorkingDir)
	preferred := *model
	if preferred == "" {
		preferred, _ = core.Repo.GetSetting(ctx, constants.SettingDefaultModel)
		if preferred != "" {
			if _, err := core.ChatService.ResolveLLMConfig(ctx, preferred); err != nil {
				preferred = ""
			}
		}
	}
	acpagent.Serve(ctx, input, protocolOutput, acpagent.Options{
		Chat: core.ChatService, Sessions: core.SessionService, Models: core.LLMConfigService, Uploads: core.ChatUpload,
		PreferredModel: preferred, ApprovalMode: *approval, Version: buildversion.Info().Version,
	}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	return nil
}
