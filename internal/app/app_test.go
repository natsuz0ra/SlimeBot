package app

import (
	"context"
	"strings"
	"testing"

	"slimebot/internal/config"
)

func TestAppConstructorReleasesCoreWhenTokenManagerFails(t *testing.T) {
	for name, construct := range map[string]func(config.Config) (*App, error){
		"server":   New,
		"headless": NewHeadless,
	} {
		t.Run(name, func(t *testing.T) {
			cfg := testConfig(t)
			cfg.JWTExpireMinutes = 0
			app, err := construct(cfg)
			if app != nil {
				app.Close(context.Background())
			}
			if err == nil || !strings.Contains(err.Error(), "jwt expire") {
				t.Fatalf("constructor error = %v, want invalid JWT expiry", err)
			}

			cfg.JWTExpireMinutes = 60
			app, err = construct(cfg)
			if err != nil {
				t.Fatalf("could not reopen the workspace after construction failed: %v", err)
			}
			app.Close(context.Background())
		})
	}
}
