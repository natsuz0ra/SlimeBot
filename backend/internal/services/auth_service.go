package services

import (
	"strings"

	"slimebot/backend/internal/auth"
	"slimebot/backend/internal/consts"
	"slimebot/backend/internal/repositories"
)

type AuthService struct {
	store repositories.SettingsReaderWriter
}

func NewAuthService(store repositories.SettingsReaderWriter) *AuthService {
	return &AuthService{store: store}
}

func (s *AuthService) VerifyLogin(username, password string) (bool, error) {
	storedUsername, err := s.store.GetSetting(consts.SettingAuthUsername)
	if err != nil {
		return false, err
	}
	storedHash, err := s.store.GetSetting(consts.SettingAuthPasswordHash)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(storedUsername) == "" || strings.TrimSpace(storedHash) == "" {
		return false, nil
	}
	if strings.TrimSpace(username) != storedUsername {
		return false, nil
	}
	return auth.ComparePassword(storedHash, password), nil
}

func (s *AuthService) MustChangePassword() (bool, error) {
	return s.store.GetSettingBool(consts.SettingAuthForcePasswordChange, false)
}

func (s *AuthService) UpdateAccount(username, oldPassword, newPassword string) error {
	newUsername := strings.TrimSpace(username)
	newPass := strings.TrimSpace(newPassword)
	if newUsername != "" {
		if err := s.store.SetSetting(consts.SettingAuthUsername, newUsername); err != nil {
			return err
		}
	}
	if newPass == "" {
		return nil
	}
	storedHash, err := s.store.GetSetting(consts.SettingAuthPasswordHash)
	if err != nil {
		return err
	}
	if strings.TrimSpace(storedHash) == "" {
		return ErrAccountNotInitialized
	}
	if strings.TrimSpace(oldPassword) == "" {
		return ErrOldPasswordRequired
	}
	if !auth.ComparePassword(storedHash, oldPassword) {
		return ErrOldPasswordInvalid
	}
	if auth.ComparePassword(storedHash, newPass) {
		return ErrPasswordUnchanged
	}
	hashed, err := auth.HashPassword(newPass)
	if err != nil {
		return err
	}
	if err := s.store.SetSetting(consts.SettingAuthPasswordHash, hashed); err != nil {
		return err
	}
	return s.store.SetSetting(consts.SettingAuthForcePasswordChange, "false")
}
