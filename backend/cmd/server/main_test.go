package main

import (
	"testing"

	"slimebot/backend/internal/config"
)

func TestValidateConfig_RejectsMissingJWTSecret(t *testing.T) {
	err := validateConfig(config.Config{
		JWTSecret:        "",
		JWTExpireMinutes: 60,
	})
	if err == nil {
		t.Fatal("expected validateConfig to fail for empty JWT secret")
	}
}

func TestValidateConfig_RejectsInvalidJWTExpireMinutes(t *testing.T) {
	err := validateConfig(config.Config{
		JWTSecret:        "secret",
		JWTExpireMinutes: 0,
	})
	if err == nil {
		t.Fatal("expected validateConfig to fail for non-positive JWT_EXPIRE")
	}
}

func TestValidateConfig_AcceptsValidConfig(t *testing.T) {
	err := validateConfig(config.Config{
		JWTSecret:        "secret",
		JWTExpireMinutes: 60,
	})
	if err != nil {
		t.Fatalf("expected validateConfig success, got=%v", err)
	}
}
