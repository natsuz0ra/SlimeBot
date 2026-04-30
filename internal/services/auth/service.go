package auth

import (
	"context"
	"strings"

	coreauth "slimebot/internal/auth"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

// AuthService handles login and account updates.
type AuthService struct {
	store domain.SettingsReaderWriter
}

// NewAuthService constructs an AuthService.
func NewAuthService(store domain.SettingsReaderWriter) *AuthService {
	return &AuthService{store: store}
}

// VerifyLogin checks username/password against stored credentials.
func (s *AuthService) VerifyLogin(ctx context.Context, username, password string) (bool, error) {
	storedUsername, err := s.store.GetSetting(ctx, constants.SettingAuthUsername)
	if err != nil {
		return false, err
	}
	storedHash, err := s.store.GetSetting(ctx, constants.SettingAuthPasswordHash)
	if err != nil {
		return false, err
	}
	storedUsername = strings.TrimSpace(storedUsername)
	storedHash = strings.TrimSpace(storedHash)
	if storedUsername == "" || storedHash == "" {
		return false, ErrAccountNotInitialized
	}
	if strings.TrimSpace(username) != storedUsername {
		return false, nil
	}
	return coreauth.ComparePassword(storedHash, password), nil
}

// MustChangePassword reports whether the user must change password after first login.
func (s *AuthService) MustChangePassword(ctx context.Context) (bool, error) {
	return s.store.GetSettingBool(ctx, constants.SettingAuthForcePasswordChange, false)
}

// UpdateAccount changes username or password; password changes require the old password.
func (s *AuthService) UpdateAccount(ctx context.Context, username, oldPassword, newPassword string) error {
	newUsername := strings.TrimSpace(username)
	newPass := strings.TrimSpace(newPassword)
	if newUsername != "" {
		if err := s.store.SetSetting(ctx, constants.SettingAuthUsername, newUsername); err != nil {
			return err
		}
	}
	if newPass == "" {
		return nil
	}
	storedHash, err := s.store.GetSetting(ctx, constants.SettingAuthPasswordHash)
	if err != nil {
		return err
	}
	if strings.TrimSpace(storedHash) == "" {
		return ErrAccountNotInitialized
	}
	if strings.TrimSpace(oldPassword) == "" {
		return ErrOldPasswordRequired
	}
	if !coreauth.ComparePassword(storedHash, oldPassword) {
		return ErrOldPasswordInvalid
	}
	if coreauth.ComparePassword(storedHash, newPass) {
		return ErrPasswordUnchanged
	}
	hashed, err := coreauth.HashPassword(newPass)
	if err != nil {
		return err
	}
	if err := s.store.SetSetting(ctx, constants.SettingAuthPasswordHash, hashed); err != nil {
		return err
	}
	return s.store.SetSetting(ctx, constants.SettingAuthForcePasswordChange, "false")
}

// EnsureDefaultAdmin seeds a default admin when no account exists and forces password change.
func (s *AuthService) EnsureDefaultAdmin() error {
	ctx := context.Background()
	username, err := s.store.GetSetting(ctx, constants.SettingAuthUsername)
	if err != nil {
		return err
	}
	passwordHash, err := s.store.GetSetting(ctx, constants.SettingAuthPasswordHash)
	if err != nil {
		return err
	}

	if strings.TrimSpace(username) == "" || strings.TrimSpace(passwordHash) == "" {
		defaultHash, hashErr := coreauth.HashPassword("admin")
		if hashErr != nil {
			return hashErr
		}
		if err := s.store.SetSetting(ctx, constants.SettingAuthUsername, "admin"); err != nil {
			return err
		}
		if err := s.store.SetSetting(ctx, constants.SettingAuthPasswordHash, defaultHash); err != nil {
			return err
		}
		return s.store.SetSetting(ctx, constants.SettingAuthForcePasswordChange, "true")
	}

	forceFlag, err := s.store.GetSetting(ctx, constants.SettingAuthForcePasswordChange)
	if err != nil {
		return err
	}
	if strings.TrimSpace(forceFlag) == "" {
		return s.store.SetSetting(ctx, constants.SettingAuthForcePasswordChange, "false")
	}
	return nil
}
