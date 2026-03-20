package auth

import (
	"strings"

	coreauth "slimebot/backend/internal/auth"
	"slimebot/backend/internal/constants"
	"slimebot/backend/internal/domain"
)

type AuthService struct {
	store domain.SettingsReaderWriter
}

func NewAuthService(store domain.SettingsReaderWriter) *AuthService {
	return &AuthService{store: store}
}

func (s *AuthService) VerifyLogin(username, password string) (bool, error) {
	storedUsername, err := s.store.GetSetting(constants.SettingAuthUsername)
	if err != nil {
		return false, err
	}
	storedHash, err := s.store.GetSetting(constants.SettingAuthPasswordHash)
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

func (s *AuthService) MustChangePassword() (bool, error) {
	return s.store.GetSettingBool(constants.SettingAuthForcePasswordChange, false)
}

func (s *AuthService) UpdateAccount(username, oldPassword, newPassword string) error {
	newUsername := strings.TrimSpace(username)
	newPass := strings.TrimSpace(newPassword)
	if newUsername != "" {
		if err := s.store.SetSetting(constants.SettingAuthUsername, newUsername); err != nil {
			return err
		}
	}
	if newPass == "" {
		return nil
	}
	storedHash, err := s.store.GetSetting(constants.SettingAuthPasswordHash)
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
	if err := s.store.SetSetting(constants.SettingAuthPasswordHash, hashed); err != nil {
		return err
	}
	return s.store.SetSetting(constants.SettingAuthForcePasswordChange, "false")
}

func (s *AuthService) EnsureDefaultAdmin() error {
	username, err := s.store.GetSetting(constants.SettingAuthUsername)
	if err != nil {
		return err
	}
	passwordHash, err := s.store.GetSetting(constants.SettingAuthPasswordHash)
	if err != nil {
		return err
	}

	if strings.TrimSpace(username) == "" || strings.TrimSpace(passwordHash) == "" {
		defaultHash, hashErr := coreauth.HashPassword("admin")
		if hashErr != nil {
			return hashErr
		}
		if err := s.store.SetSetting(constants.SettingAuthUsername, "admin"); err != nil {
			return err
		}
		if err := s.store.SetSetting(constants.SettingAuthPasswordHash, defaultHash); err != nil {
			return err
		}
		return s.store.SetSetting(constants.SettingAuthForcePasswordChange, "true")
	}

	forceFlag, err := s.store.GetSetting(constants.SettingAuthForcePasswordChange)
	if err != nil {
		return err
	}
	if strings.TrimSpace(forceFlag) == "" {
		return s.store.SetSetting(constants.SettingAuthForcePasswordChange, "false")
	}
	return nil
}
