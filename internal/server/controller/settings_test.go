package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slimebot/internal/apperrors"
	agentssvc "slimebot/internal/services/agents"
	settingssvc "slimebot/internal/services/settings"
	"testing"
)

type settingsServiceStub struct {
	settings    *settingssvc.AppSettings
	input       settingssvc.UpdateSettingsInput
	proxy       string
	proxyResult *settingssvc.ProxyTestResult
	proxyError  error
}

type agentsInstructionsServiceStub struct {
	content string
	path    string
	updated string
}

func (s *settingsServiceStub) Get(_ context.Context) (*settingssvc.AppSettings, error) {
	return s.settings, nil
}

func (s *settingsServiceStub) Update(_ context.Context, input settingssvc.UpdateSettingsInput) error {
	s.input = input
	return nil
}

func (s *settingsServiceStub) TestProxy(_ context.Context, raw string) (*settingssvc.ProxyTestResult, error) {
	s.proxy = raw
	return s.proxyResult, s.proxyError
}

func TestProxyConnectionReturnsDiagnosticsAndValidationErrors(t *testing.T) {
	stub := &settingsServiceStub{proxyResult: &settingssvc.ProxyTestResult{
		Success: false, Route: "proxy", TargetURL: "https://api.github.com", LatencyMs: 12, ErrorCode: "refused",
	}}
	handler := NewHTTPController(nil, nil, stub, nil, nil, nil, nil, nil, nil, nil, nil)
	for _, test := range []struct {
		payload string
		err     error
		status  int
	}{
		{`{"proxyUrl":"http://127.0.0.1:7890"}`, nil, http.StatusOK},
		{`{"proxyUrl":"invalid"}`, apperrors.ErrInvalidInput, http.StatusBadRequest},
		{`{"proxyUrl":123}`, nil, http.StatusBadRequest},
	} {
		stub.proxyError = test.err
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/settings/proxy/test", bytes.NewBufferString(test.payload))
		handler.TestProxyConnection(NewChiContext(response, request))
		if response.Code != test.status {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		if test.status == http.StatusOK {
			var result settingssvc.ProxyTestResult
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.ErrorCode != "refused" || result.Route != "proxy" || stub.proxy != "http://127.0.0.1:7890" {
				t.Fatalf("diagnostic response = %+v, %v", result, err)
			}
		}
	}
}

func (s *agentsInstructionsServiceStub) ReadGlobal(_ context.Context) (agentssvc.File, error) {
	return agentssvc.File{Content: s.content, Path: s.path}, nil
}

func (s *agentsInstructionsServiceStub) UpdateGlobal(_ context.Context, content string) error {
	s.updated = content
	return nil
}

func TestGetSettingsReturnsCLISandboxFields(t *testing.T) {
	settingsStub := &settingsServiceStub{settings: &settingssvc.AppSettings{
		Language:                        "zh-CN",
		SandboxMode:                     "danger-full-access",
		SandboxNetworkEnabled:           true,
		SandboxWritableRoots:            []string{"/tmp/web"},
		SandboxNetworkAllowedDomains:    []string{"web.example.com"},
		CLISandboxMode:                  "workspace-write",
		CLISandboxNetworkEnabled:        false,
		CLISandboxWritableRoots:         []string{"/tmp/cli"},
		CLISandboxNetworkAllowedDomains: []string{"cli.example.com"},
	}}
	controller := NewHTTPController(nil, nil, settingsStub, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	resp := httptest.NewRecorder()
	controller.GetSettings(NewChiContext(resp, req))

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["sandboxMode"] != "danger-full-access" || body["cliSandboxMode"] != "workspace-write" {
		t.Fatalf("unexpected sandbox modes: %#v", body)
	}
	if body["cliSandboxNetworkEnabled"] != false {
		t.Fatalf("unexpected CLI network flag: %#v", body)
	}
}

func TestUpdateSettingsAcceptsCLISandboxFields(t *testing.T) {
	settingsStub := &settingsServiceStub{}
	controller := NewHTTPController(nil, nil, settingsStub, nil, nil, nil, nil, nil, nil, nil, nil)
	body := bytes.NewBufferString(`{
		"cliSandboxMode":"read-only",
		"cliSandboxWritableRoots":["/tmp/cli"],
		"cliSandboxNetworkEnabled":false,
		"cliSandboxNetworkAllowedDomains":["cli.example.com"]
	}`)

	req := httptest.NewRequest(http.MethodPut, "/settings", body)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	controller.UpdateSettings(NewChiContext(resp, req))

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if settingsStub.input.CLISandboxMode == nil || *settingsStub.input.CLISandboxMode != "read-only" {
		t.Fatalf("cli sandbox mode input = %#v", settingsStub.input.CLISandboxMode)
	}
	if settingsStub.input.CLISandboxNetworkEnabled == nil || *settingsStub.input.CLISandboxNetworkEnabled != false {
		t.Fatalf("cli sandbox network input = %#v", settingsStub.input.CLISandboxNetworkEnabled)
	}
	if settingsStub.input.CLISandboxWritableRoots == nil || len(*settingsStub.input.CLISandboxWritableRoots) != 1 || (*settingsStub.input.CLISandboxWritableRoots)[0] != "/tmp/cli" {
		t.Fatalf("cli sandbox roots input = %#v", settingsStub.input.CLISandboxWritableRoots)
	}
	if settingsStub.input.CLISandboxNetworkAllowedDomains == nil || len(*settingsStub.input.CLISandboxNetworkAllowedDomains) != 1 || (*settingsStub.input.CLISandboxNetworkAllowedDomains)[0] != "cli.example.com" {
		t.Fatalf("cli sandbox domains input = %#v", settingsStub.input.CLISandboxNetworkAllowedDomains)
	}
}

func TestGetAgentsInstructionsReturnsContentAndPath(t *testing.T) {
	agentsStub := &agentsInstructionsServiceStub{content: "全局规则", path: "/tmp/AGENTS.md"}
	controller := NewHTTPController(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	controller.SetAgentsInstructionsService(agentsStub)

	req := httptest.NewRequest(http.MethodGet, "/agents-instructions", nil)
	resp := httptest.NewRecorder()
	controller.GetAgentsInstructions(NewChiContext(resp, req))

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["content"] != "全局规则" || body["path"] != "/tmp/AGENTS.md" {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestUpdateAgentsInstructionsPersistsContent(t *testing.T) {
	agentsStub := &agentsInstructionsServiceStub{}
	controller := NewHTTPController(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	controller.SetAgentsInstructionsService(agentsStub)
	body := bytes.NewBufferString(`{"content":"新的规则"}`)

	req := httptest.NewRequest(http.MethodPut, "/agents-instructions", body)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	controller.UpdateAgentsInstructions(NewChiContext(resp, req))

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if agentsStub.updated != "新的规则" {
		t.Fatalf("updated content = %q", agentsStub.updated)
	}
}
