package config

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
	sessionsvc "slimebot/internal/services/session"

	"github.com/google/uuid"
)

type ACPConfigStore interface {
	ListACPConfigs(ctx context.Context) ([]domain.ACPConfig, error)
	GetACPConfig(ctx context.Context, id string) (domain.ACPConfig, error)
	CreateACPConfig(ctx context.Context, value *domain.ACPConfig) error
	UpdateACPConfig(ctx context.Context, value *domain.ACPConfig) error
	DeleteACPConfig(ctx context.Context, id string) error
}

type ACPConfigInput struct {
	Name          *string           `json:"name"`
	Command       *string           `json:"command"`
	Args          *[]string         `json:"args"`
	Env           map[string]string `json:"env"`
	RemoveEnvKeys []string          `json:"removeEnvKeys"`
	DefaultCwd    *string           `json:"defaultCwd"`
	TimeoutSec    *int              `json:"timeoutSec"`
	IsEnabled     *bool             `json:"isEnabled"`
}

type ACPConfigService struct{ store ACPConfigStore }

func NewACPConfigService(store ACPConfigStore) *ACPConfigService {
	return &ACPConfigService{store: store}
}

var acpEnvKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func ACPProcessValues(item domain.ACPConfig) ([]string, map[string]string, error) {
	args := make([]string, 0)
	env := make(map[string]string)
	if item.ArgsJSON != "" {
		if err := json.Unmarshal([]byte(item.ArgsJSON), &args); err != nil {
			return nil, nil, err
		}
	}
	if item.EnvJSON != "" {
		if err := json.Unmarshal([]byte(item.EnvJSON), &env); err != nil {
			return nil, nil, err
		}
	}
	if args == nil {
		args = []string{}
	}
	if env == nil {
		env = map[string]string{}
	}
	return args, env, nil
}

func acpPublicConfig(item domain.ACPConfig) (domain.ACPConfig, error) {
	args, env, err := ACPProcessValues(item)
	item.Args = args
	item.EnvKeys = slices.Sorted(maps.Keys(env))
	item.EnvJSON = ""
	return item, err
}

func (s *ACPConfigService) List(ctx context.Context) ([]domain.ACPConfig, error) {
	items, err := s.store.ListACPConfigs(ctx)
	if err != nil {
		return nil, err
	}
	for i, item := range items {
		items[i], err = acpPublicConfig(item)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *ACPConfigService) Create(ctx context.Context, input ACPConfigInput) (domain.ACPConfig, error) {
	value := domain.ACPConfig{ID: uuid.NewString(), TimeoutSec: 300, IsEnabled: true}
	if err := applyACPInput(&value, input); err != nil {
		return domain.ACPConfig{}, err
	}
	if err := s.store.CreateACPConfig(ctx, &value); err != nil {
		return domain.ACPConfig{}, err
	}
	return acpPublicConfig(value)
}

func (s *ACPConfigService) Update(ctx context.Context, id string, input ACPConfigInput) (domain.ACPConfig, error) {
	value, err := s.store.GetACPConfig(ctx, id)
	if err != nil {
		return domain.ACPConfig{}, err
	}
	if err := applyACPInput(&value, input); err != nil {
		return domain.ACPConfig{}, err
	}
	if err := s.store.UpdateACPConfig(ctx, &value); err != nil {
		return domain.ACPConfig{}, err
	}
	return acpPublicConfig(value)
}

func (s *ACPConfigService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteACPConfig(ctx, id)
}

func applyACPInput(value *domain.ACPConfig, input ACPConfigInput) error {
	args, env, err := ACPProcessValues(*value)
	if err != nil {
		return err
	}
	if input.Name != nil {
		value.Name = strings.TrimSpace(*input.Name)
	}
	if input.Command != nil {
		value.Command = strings.TrimSpace(*input.Command)
	}
	if input.Args != nil {
		args = *input.Args
	}
	if input.DefaultCwd != nil {
		value.DefaultCwd = strings.TrimSpace(*input.DefaultCwd)
		if value.DefaultCwd != "" {
			cwd, err := sessionsvc.ValidateWorkingDirectory(value.DefaultCwd)
			if err != nil {
				return fmt.Errorf("%w: invalid default working directory", apperrors.ErrInvalidInput)
			}
			value.DefaultCwd = cwd
		}
	}
	if input.TimeoutSec != nil {
		value.TimeoutSec = *input.TimeoutSec
	}
	if input.IsEnabled != nil {
		value.IsEnabled = *input.IsEnabled
	}
	for _, key := range input.RemoveEnvKeys {
		delete(env, key)
	}
	maps.Copy(env, input.Env)
	if value.Name == "" || len(value.Name) > 128 || value.Command == "" || len(value.Command) > 512 ||
		strings.ContainsAny(value.Command, "\x00\r\n") || len(value.DefaultCwd) > 2048 || value.TimeoutSec < 1 || value.TimeoutSec > 3600 ||
		len(args) > 128 || len(env) > 128 {
		return fmt.Errorf("%w: invalid ACP configuration fields", apperrors.ErrInvalidInput)
	}
	for _, arg := range args {
		if len(arg) > 8192 || strings.ContainsRune(arg, 0) {
			return fmt.Errorf("%w: invalid ACP argument", apperrors.ErrInvalidInput)
		}
	}
	for key, text := range env {
		if !acpEnvKey.MatchString(key) || len(text) > 16384 || strings.ContainsRune(text, 0) {
			return fmt.Errorf("%w: invalid ACP environment variable", apperrors.ErrInvalidInput)
		}
	}
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		return err
	}
	encodedEnv, err := json.Marshal(env)
	if err != nil {
		return err
	}
	value.ArgsJSON, value.EnvJSON = string(encodedArgs), string(encodedEnv)
	return nil
}
