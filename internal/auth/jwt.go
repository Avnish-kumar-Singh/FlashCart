package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid or expired token")

type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

type Claims struct {
	UserID string    `json:"uid"`
	Role   string    `json:"role"`
	Type   TokenType `json:"typ"`
	jwt.RegisteredClaims
}

type Issuer struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewIssuer(secret string, accessTTL, refreshTTL time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), accessTokenTTL: accessTTL, refreshTokenTTL: refreshTTL}
}

// IssuePair returns (accessToken, refreshToken, error).
//
// The access token is short-lived and sent on every request; the refresh
// token is long-lived and only used to mint new access tokens. This split
// means a stolen access token has a small blast radius, and refresh tokens
// can be revoked (Phase 3+: store a revocation set in Redis) without
// forcing a full re-login on every short-lived token expiry.
func (i *Issuer) IssuePair(userID, role string) (string, string, error) {
	access, err := i.issue(userID, role, AccessToken, i.accessTokenTTL)
	if err != nil {
		return "", "", err
	}
	refresh, err := i.issue(userID, role, RefreshToken, i.refreshTokenTTL)
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

func (i *Issuer) issue(userID, role string, typ TokenType, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		Type:   typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(i.secret)
}

// Parse validates signature + expiry and returns the claims.
func (i *Issuer) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return i.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ParseRefresh validates a token AND checks it's specifically a refresh token,
// so an access token can't be replayed against the refresh endpoint.
func (i *Issuer) ParseRefresh(tokenString string) (*Claims, error) {
	claims, err := i.Parse(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.Type != RefreshToken {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
