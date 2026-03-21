package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Claims 定义 JWT 中承载的业务字段与标准声明。
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// TokenManager 负责 JWT 的签发与解析。
type TokenManager struct {
	secret        []byte
	expireMinutes int
}

// NewTokenManager 创建 JWT 管理器并校验关键配置。
func NewTokenManager(secret string, expireMinutes int) (*TokenManager, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("jwt secret cannot be empty")
	}
	if expireMinutes <= 0 {
		return nil, errors.New("jwt expire must be greater than 0")
	}
	return &TokenManager{
		secret:        []byte(secret),
		expireMinutes: expireMinutes,
	}, nil
}

// Generate 为指定用户名生成带过期时间的 JWT。
func (m *TokenManager) Generate(username string) (string, error) {
	now := time.Now()
	claims := Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(m.expireMinutes) * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Parse 校验并解析 JWT，返回业务 Claims。
func (m *TokenManager) Parse(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unsupported signing algorithm: %v", token.Method.Alg())
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// ExpireMinutes 返回当前 token 过期分钟数配置。
func (m *TokenManager) ExpireMinutes() int {
	return m.expireMinutes
}

// HashPassword 使用 bcrypt 对密码做哈希存储。
func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// ComparePassword 校验明文密码与 bcrypt 哈希是否匹配。
func ComparePassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
