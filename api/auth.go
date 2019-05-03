package api

import (
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
	"time"
)

const (
	headerAuth   = "Authorization"
	headerBearer = "Bearer"
	tokenExpirey = time.Hour * 6
)

type TokenClaims struct {
	jwt.StandardClaims

	IsAdmin bool
	IsMod   bool
	IsRO    bool
}

func AuthMiddleware(next http.HandlerFunc, requiresAdmin, requiresMod bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isValidToken(r, requiresAdmin, requiresMod) {
			w.WriteHeader(http.StatusUnauthorized)
			writeResponse(w, nil)
			return
		}

		next(w, r)
	})
}

func TokenHandler(w http.ResponseWriter, r *http.Request) {
	writeResponse(w, nil)
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	req := struct {
		Password string `json:"password"`
	}{}

	if err := decodeJSON(r, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	isAdmin := req.Password == adminPassword
	isMod := req.Password == modPassword || isAdmin
	isRO := req.Password == roPassword || isMod

	if !isAdmin && !isMod && !isRO {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	token, expires, err := newToken(isAdmin, isMod, isRO)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := struct {
		Token    string    `json:"token"`
		Expires  time.Time `json:"expires"`
		Admin    bool      `json:"admin"`
		Mod      bool      `json:"mod"`
		ReadOnly bool      `json:"read_only"`
	}{token, expires, isAdmin, isMod, isRO}
	writeResponse(w, &resp)
}

func newToken(isAdmin, isMod, isRO bool) (string, time.Time, error) {
	exp := time.Now().Add(tokenExpirey)
	now := time.Now()
	claims := TokenClaims{jwt.StandardClaims{
		ExpiresAt: exp.Unix(),
		IssuedAt:  now.Unix(),
		NotBefore: now.Unix(),
	},
		isAdmin,
		isMod,
		isRO,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(signKey)

	if err != nil {
		logrus.WithField("err", err).Error("Error generating token")
		return "", time.Time{}, err
	}

	return tokenString, exp, nil
}

func isValidToken(r *http.Request, requiresAdmin, requiresMod bool) bool {
	bearer := strings.Split(r.Header.Get(headerAuth), " ")

	if len(bearer) != 2 || bearer[0] != headerBearer {
		return false
	}

	tokenString := bearer[1]
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("Unexpected token signing method: %v", token.Header["alg"])
		}

		return verifyKey, nil
	})

	if err != nil {
		return false
	}

	if claims, ok := token.Claims.(*TokenClaims); ok && token.Valid {
		if requiresAdmin && !claims.IsAdmin || requiresMod && !claims.IsMod {
			return false
		}
	} else {
		return false
	}

	return true
}