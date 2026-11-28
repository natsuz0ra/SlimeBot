package servicecontrol

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	kservice "github.com/kardianos/service"

	"slimebot/internal/app"
	"slimebot/internal/config"
	"slimebot/internal/runtime"
)

const (
	serviceName        = "slimebot"
	serviceDisplayName = "SlimeBot"
	serviceDescription = "SlimeBot web service"
)

type Controller struct {
	service serviceBackend
}

func NewController() (*Controller, error) {
	prg := &program{}
	svc, err := newServiceBackend(prg, serviceConfig())
	if err != nil {
		return nil, err
	}
	return &Controller{service: svc}, nil
}

func (c *Controller) Install() error {
	return c.service.Install()
}

func (c *Controller) Start() error {
	return c.service.Start()
}

func (c *Controller) Stop() error {
	return c.service.Stop()
}

func (c *Controller) Restart() error {
	return c.service.Restart()
}

func (c *Controller) Status() (string, error) {
	return c.service.Status()
}

func (c *Controller) Uninstall() error {
	return c.service.Uninstall()
}

func (c *Controller) Run() error {
	if err := runtime.EnsureAndLoadEnv(); err != nil {
		return err
	}
	return c.service.Run()
}

type serviceBackend interface {
	Install() error
	Start() error
	Stop() error
	Restart() error
	Status() (string, error)
	Uninstall() error
	Run() error
}

type program struct {
	cancel context.CancelFunc
	done   chan error
}

func (p *program) Start(_ kservice.Service) error {
	cfg, err := config.LoadValidated()
	if err != nil {
		return err
	}
	if err := app.ValidateConfig(cfg); err != nil {
		return err
	}
	slimeApp, err := app.New(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan error, 1)

	go func() {
		p.done <- slimeApp.Run(ctx)
	}()

	return nil
}

func (p *program) Stop(_ kservice.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.done == nil {
		return nil
	}
	select {
	case err := <-p.done:
		return err
	case <-time.After(15 * time.Second):
		return fmt.Errorf("timed out waiting for %s to stop", serviceName)
	}
}

func serviceConfig() *kservice.Config {
	return &kservice.Config{
		Name:        serviceName,
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
		Arguments:   []string{"service", "run"},
		Executable:  executablePath(),
		Option: kservice.KeyValue{
			"RunAtLoad":   true,
			"Restart":     "on-failure",
			"StartType":   "manual",
			"UserService": true,
		},
	}
}

func executablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	abs, err := filepath.Abs(exe)
	if err != nil {
		return exe
	}
	return abs
}
