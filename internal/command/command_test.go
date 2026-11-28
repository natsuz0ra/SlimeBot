package command

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestExecuteDefaultsToCLI(t *testing.T) {
	var called []string

	err := Execute(Options{
		Args: []string{},
		RunCLI: func() error {
			called = append(called, "cli")
			return nil
		},
		RunServer: func() error {
			called = append(called, "server")
			return nil
		},
		Service: &fakeServiceController{},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got := strings.Join(called, ","); got != "cli" {
		t.Fatalf("expected default command to run CLI, got %q", got)
	}
}

func TestExecuteRoutesServer(t *testing.T) {
	var called []string
	err := Execute(Options{
		Args: []string{"server"},
		RunCLI: func() error {
			called = append(called, "cli")
			return nil
		},
		RunServer: func() error {
			called = append(called, "server")
			return nil
		},
		Service: &fakeServiceController{},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got := strings.Join(called, ","); got != "server" {
		t.Fatalf("expected server, got %q", got)
	}
}

func TestExecuteRoutesServiceActions(t *testing.T) {
	actions := []string{"install", "stop", "restart", "status", "uninstall"}

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			service := &fakeServiceController{}
			var stdout bytes.Buffer
			err := Execute(Options{
				Args:      []string{"service", action},
				RunCLI:    func() error { return errors.New("cli should not run") },
				RunServer: func() error { return errors.New("server should not run") },
				Service:   service,
				Stdout:    &stdout,
			})
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if got := strings.Join(service.calls, ","); got != action {
				t.Fatalf("expected service action %q, got %q", action, got)
			}
			if action == "status" && !strings.Contains(stdout.String(), "running") {
				t.Fatalf("expected status output, got %q", stdout.String())
			}
		})
	}
}

func TestExecuteVersionAndHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "version", args: []string{"version"}, want: "1.2.3"},
		{name: "help", args: []string{"help"}, want: "slimebot service install"},
		{name: "dash help", args: []string{"--help"}, want: "slimebot service install"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := Execute(Options{
				Args:      tt.args,
				RunCLI:    func() error { return errors.New("cli should not run") },
				RunServer: func() error { return errors.New("server should not run") },
				Service:   &fakeServiceController{},
				Stdout:    &stdout,
				Version: VersionInfo{
					Version: "1.2.3",
					Commit:  "abc123",
					Date:    "2026-05-17",
				},
			})
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if !strings.Contains(stdout.String(), tt.want) {
				t.Fatalf("expected output to contain %q, got %q", tt.want, stdout.String())
			}
		})
	}
}

func TestHelpOmitsUpdateYesFlag(t *testing.T) {
	help := HelpText()
	if strings.Contains(help, "update --yes") {
		t.Fatalf("help should not mention update --yes:\n%s", help)
	}
	if !strings.Contains(help, "slimebot update") || !strings.Contains(help, "Update to the latest release") {
		t.Fatalf("help should show bare update command:\n%s", help)
	}
}

func TestHelpOmitsExplicitCLICommandAndDescribesVersionUpdate(t *testing.T) {
	help := HelpText()
	if strings.Contains(help, "slimebot cli") {
		t.Fatalf("help should not mention explicit cli command:\n%s", help)
	}
	if !strings.Contains(help, "slimebot update --version vX.Y.Z  Update to a specific release") {
		t.Fatalf("help should describe version update command:\n%s", help)
	}
}

func TestExecuteRoutesUpdateCommand(t *testing.T) {
	var calledArgs []string
	var stdout bytes.Buffer
	err := Execute(Options{
		Args:      []string{"update", "--check"},
		RunCLI:    func() error { return errors.New("cli should not run") },
		RunServer: func() error { return errors.New("server should not run") },
		Service:   &fakeServiceController{},
		Stdout:    &stdout,
		Update: func(args []string, w io.Writer) error {
			calledArgs = append(calledArgs, args...)
			_, _ = w.Write([]byte("update ok\n"))
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got := strings.Join(calledArgs, ","); got != "--check" {
		t.Fatalf("update args = %q, want --check", got)
	}
	if !strings.Contains(stdout.String(), "update ok") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestExecuteRoutesACPCommand(t *testing.T) {
	var calledArgs []string
	err := Execute(Options{
		Args:      []string{"acp", "--model", "model-1", "--approval-mode", "auto"},
		RunCLI:    func() error { return errors.New("cli should not run") },
		RunServer: func() error { return errors.New("server should not run") },
		RunACP: func(args []string) error {
			calledArgs = append(calledArgs, args...)
			return nil
		},
		Service: &fakeServiceController{},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got := strings.Join(calledArgs, ","); got != "--model,model-1,--approval-mode,auto" {
		t.Fatalf("ACP args = %q", got)
	}
}

func TestExecuteACPRequiresConfiguredHandler(t *testing.T) {
	err := Execute(Options{
		Args:      []string{"acp"},
		RunCLI:    func() error { return errors.New("cli should not run") },
		RunServer: func() error { return errors.New("server should not run") },
		Service:   &fakeServiceController{},
	})
	if err == nil || !strings.Contains(err.Error(), "ACP command is not configured") {
		t.Fatalf("expected missing ACP handler error, got %v", err)
	}
}

func TestHelpListsACPCommand(t *testing.T) {
	if !strings.Contains(HelpText(), "slimebot acp") {
		t.Fatalf("help should list the ACP command:\n%s", HelpText())
	}
}

func TestExecuteServiceStartPrintsLocalURLAfterHealthReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	port := testServerPort(t, server.URL)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SERVER_PORT", port)
	t.Setenv("JWT_SECRET", "test-secret")
	setServiceReadyTestTimings(t)

	var stdout bytes.Buffer
	service := &fakeServiceController{}
	err := Execute(Options{
		Args:      []string{"service", "start"},
		RunCLI:    func() error { return errors.New("cli should not run") },
		RunServer: func() error { return errors.New("server should not run") },
		Service:   service,
		Stdout:    &stdout,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got := strings.Join(service.calls, ","); got != "start" {
		t.Fatalf("expected service start, got %q", got)
	}
	if want := "SlimeBot web service is ready: http://localhost:" + port; !strings.Contains(stdout.String(), want) {
		t.Fatalf("expected ready URL %q, got %q", want, stdout.String())
	}
}

func TestExecuteServiceStartReturnsStartErrorWithoutHealthCheck(t *testing.T) {
	startErr := errors.New("start failed")
	service := &fakeServiceController{startErr: startErr}
	var stdout bytes.Buffer

	err := Execute(Options{
		Args:      []string{"service", "start"},
		RunCLI:    func() error { return errors.New("cli should not run") },
		RunServer: func() error { return errors.New("server should not run") },
		Service:   service,
		Stdout:    &stdout,
	})
	if !errors.Is(err, startErr) {
		t.Fatalf("expected start error, got %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
}

func TestExecuteServiceStartFailsWhenHealthDoesNotBecomeReady(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SERVER_PORT", "1")
	t.Setenv("JWT_SECRET", "test-secret")
	setServiceReadyTestTimings(t)

	var stdout bytes.Buffer
	err := Execute(Options{
		Args:      []string{"service", "start"},
		RunCLI:    func() error { return errors.New("cli should not run") },
		RunServer: func() error { return errors.New("server should not run") },
		Service:   &fakeServiceController{},
		Stdout:    &stdout,
	})
	if err == nil {
		t.Fatal("expected readiness timeout error")
	}
	if !strings.Contains(err.Error(), "service start succeeded but health check did not become ready") {
		t.Fatalf("expected readiness error, got %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
}

func TestExecuteRejectsUnknownCommand(t *testing.T) {
	tests := []string{"wat"}
	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			err := Execute(Options{
				Args:      []string{command},
				RunCLI:    func() error { return nil },
				RunServer: func() error { return nil },
				Service:   &fakeServiceController{},
			})
			if err == nil {
				t.Fatal("expected unknown command to fail")
			}
			if !strings.Contains(err.Error(), "unknown command") {
				t.Fatalf("expected unknown command error, got %v", err)
			}
		})
	}
}

func TestExecuteRejectsUnknownServiceAction(t *testing.T) {
	err := Execute(Options{
		Args:      []string{"service", "bounce"},
		RunCLI:    func() error { return nil },
		RunServer: func() error { return nil },
		Service:   &fakeServiceController{},
	})
	if err == nil {
		t.Fatal("expected unknown service action to fail")
	}
	if !strings.Contains(err.Error(), "unknown service action") {
		t.Fatalf("expected unknown service action error, got %v", err)
	}
}

type fakeServiceController struct {
	calls    []string
	startErr error
}

func (f *fakeServiceController) Install() error {
	f.calls = append(f.calls, "install")
	return nil
}

func (f *fakeServiceController) Start() error {
	f.calls = append(f.calls, "start")
	return f.startErr
}

func (f *fakeServiceController) Stop() error {
	f.calls = append(f.calls, "stop")
	return nil
}

func (f *fakeServiceController) Restart() error {
	f.calls = append(f.calls, "restart")
	return nil
}

func (f *fakeServiceController) Status() (string, error) {
	f.calls = append(f.calls, "status")
	return "running", nil
}

func (f *fakeServiceController) Uninstall() error {
	f.calls = append(f.calls, "uninstall")
	return nil
}

func (f *fakeServiceController) Run() error {
	f.calls = append(f.calls, "run")
	return nil
}

func testServerPort(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	return parsed.Port()
}

func setServiceReadyTestTimings(t *testing.T) {
	t.Helper()
	oldTimeout := serviceReadyTimeout
	oldInterval := serviceReadyInterval
	oldHTTPTimeout := serviceReadyHTTPTimeout
	serviceReadyTimeout = 80 * time.Millisecond
	serviceReadyInterval = 5 * time.Millisecond
	serviceReadyHTTPTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		serviceReadyTimeout = oldTimeout
		serviceReadyInterval = oldInterval
		serviceReadyHTTPTimeout = oldHTTPTimeout
	})
}
