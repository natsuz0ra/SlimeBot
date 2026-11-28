package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Claims carries app fields plus standard JWT registered claims.
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// TokenManager signs and parses JWTs.
type TokenManager struct {
	secret        []byte
	expireMinutes int
}

// NewTokenManager constructs a TokenManager after validating secret and TTL.
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

// Generate issues a JWT for the given username with configured expiry.
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

// Parse validates a JWT string and returns Claims.
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
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	claims := token.Claims.(*Claims)
	return claims, nil
}

// ExpireMinutes returns configured token lifetime in minutes.
func (m *TokenManager) ExpireMinutes() int {
	return m.expireMinutes
}

// HashPassword hashes a password with bcrypt for storage.
func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// ComparePassword checks plaintext against a bcrypt hash.
func ComparePassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
