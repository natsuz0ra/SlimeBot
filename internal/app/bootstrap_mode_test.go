package app

import (
	"testing"

	"slimebot/internal/config"
)

func TestValidateConfigForMode_CLIAllowsEmptyJWT(t *testing.T) {
	err := ValidateConfigForMode(config.Config{
		JWTSecret:        "",
		JWTExpireMinutes: 0,
	}, RunModeCLI)
	if err != nil {
		t.Fatalf("cli mode should allow empty jwt config, got: %v", err)
	}
}

func TestValidateConfigForMode_ServerStillRequiresJWT(t *testing.T) {
	err := ValidateConfigForMode(config.Config{
		JWTSecret:        "",
		JWTExpireMinutes: 60,
	}, RunModeServer)
	if err == nil {
		t.Fatal("server mode should require JWT_SECRET")
	}
}
