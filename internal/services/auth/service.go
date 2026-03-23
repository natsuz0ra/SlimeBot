package auth

import (
	"context"
	"strings"

	coreauth "slimebot/internal/auth"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

// AuthService 封装账号鉴权与账户设置相关逻辑。
type AuthService struct {
	store domain.SettingsReaderWriter
}

// NewAuthService 创建认证服务。
func NewAuthService(store domain.SettingsReaderWriter) *AuthService {
	return &AuthService{store: store}
}

// VerifyLogin 校验用户名与密码是否匹配当前账户设置。
func (s *AuthService) VerifyLogin(username, password string) (bool, error) {
	storedUsername, err := s.store.GetSetting(context.Background(), constants.SettingAuthUsername)
	if err != nil {
		return false, err
	}
	storedHash, err := s.store.GetSetting(context.Background(), constants.SettingAuthPasswordHash)
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

// MustChangePassword 返回是否要求用户首次登录后修改密码。
func (s *AuthService) MustChangePassword() (bool, error) {
	return s.store.GetSettingBool(constants.SettingAuthForcePasswordChange, false)
}

// UpdateAccount 更新用户名或密码；修改密码时必须校验旧密码。
func (s *AuthService) UpdateAccount(username, oldPassword, newPassword string) error {
	newUsername := strings.TrimSpace(username)
	newPass := strings.TrimSpace(newPassword)
	if newUsername != "" {
		if err := s.store.SetSetting(context.Background(), constants.SettingAuthUsername, newUsername); err != nil {
			return err
		}
	}
	if newPass == "" {
		return nil
	}
	storedHash, err := s.store.GetSetting(context.Background(), constants.SettingAuthPasswordHash)
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
	if err := s.store.SetSetting(context.Background(), constants.SettingAuthPasswordHash, hashed); err != nil {
		return err
	}
	return s.store.SetSetting(context.Background(), constants.SettingAuthForcePasswordChange, "false")
}

// EnsureDefaultAdmin 在未初始化时创建默认 admin 账号并要求首次改密。
func (s *AuthService) EnsureDefaultAdmin() error {
	username, err := s.store.GetSetting(context.Background(), constants.SettingAuthUsername)
	if err != nil {
		return err
	}
	passwordHash, err := s.store.GetSetting(context.Background(), constants.SettingAuthPasswordHash)
	if err != nil {
		return err
	}

	if strings.TrimSpace(username) == "" || strings.TrimSpace(passwordHash) == "" {
		defaultHash, hashErr := coreauth.HashPassword("admin")
		if hashErr != nil {
			return hashErr
		}
		if err := s.store.SetSetting(context.Background(), constants.SettingAuthUsername, "admin"); err != nil {
			return err
		}
		if err := s.store.SetSetting(context.Background(), constants.SettingAuthPasswordHash, defaultHash); err != nil {
			return err
		}
		return s.store.SetSetting(context.Background(), constants.SettingAuthForcePasswordChange, "true")
	}

	forceFlag, err := s.store.GetSetting(context.Background(), constants.SettingAuthForcePasswordChange)
	if err != nil {
		return err
	}
	if strings.TrimSpace(forceFlag) == "" {
		return s.store.SetSetting(context.Background(), constants.SettingAuthForcePasswordChange, "false")
	}
	return nil
}
