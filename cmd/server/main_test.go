package main

import (
	"io"
	"os"
	"testing"

	"slimebot/internal/app"
	"slimebot/internal/config"
)

func TestValidateConfig_RejectsMissingJWTSecret(t *testing.T) {
	err := app.ValidateConfig(config.Config{
		JWTSecret:        "",
		JWTExpireMinutes: 60,
	})
	if err == nil {
		t.Fatal("expected validateConfig to fail for empty JWT secret")
	}
}

func TestValidateConfig_RejectsInvalidJWTExpireMinutes(t *testing.T) {
	err := app.ValidateConfig(config.Config{
		JWTSecret:        "secret",
		JWTExpireMinutes: 0,
	})
	if err == nil {
		t.Fatal("expected validateConfig to fail for non-positive JWT_EXPIRE")
	}
}

func TestValidateConfig_AcceptsValidConfig(t *testing.T) {
	err := app.ValidateConfig(config.Config{
		JWTSecret:        "secret",
		JWTExpireMinutes: 60,
	})
	if err != nil {
		t.Fatalf("expected validateConfig success, got=%v", err)
	}
}

func TestRunACPRoutesProtocolToSavedStdout(t *testing.T) {
	originalStdout := os.Stdout
	originalRunACP := runACPApp
	protocolReader, protocolWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create protocol pipe: %v", err)
	}
	defer func() {
		os.Stdout = originalStdout
		runACPApp = originalRunACP
		_ = protocolReader.Close()
		_ = protocolWriter.Close()
	}()

	os.Stdout = protocolWriter
	var received io.Writer
	var stdoutDuringRun *os.File
	runACPApp = func(_ []string, _ io.Reader, output io.Writer) error {
		received = output
		stdoutDuringRun = os.Stdout
		return nil
	}

	if err := runACP([]string{"--model", "test-model"}); err != nil {
		t.Fatalf("runACP: %v", err)
	}
	if received != protocolWriter {
		t.Fatal("expected the saved stdout to be handed to the ACP protocol stream")
	}
	if stdoutDuringRun != os.Stderr {
		t.Fatal("expected os.Stdout to point at stderr while ACP runs")
	}
	if os.Stdout != protocolWriter {
		t.Fatal("expected stdout to be restored to its value at ACP entry")
	}
}
