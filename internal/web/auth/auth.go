package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type UserAuth struct {
	credentials map[string]string
}

func FromHtpasswd(htpasswd string) (UserAuth, error) {
	credentials := make(map[string]string)
	for _, entry := range strings.Fields(htpasswd) {
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			return UserAuth{}, fmt.Errorf("htpasswd entry has no separator: %v", entry)
		}

		credentials[parts[0]] = parts[1]
	}

	return UserAuth{credentials}, nil
}

var ErrUserDoesNotExist = errors.New("user does not exist")
var ErrPasswordMismatch = errors.New("password incorrect")
var ErrBcryptError = errors.New("bcrypt error")

func (a *UserAuth) VerifyCredentials(user, password string) error {
	pwHash, exists := a.credentials[user]
	if !exists {
		return ErrUserDoesNotExist
	}

	err := bcrypt.CompareHashAndPassword([]byte(pwHash), []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrPasswordMismatch
		}

		return ErrBcryptError
	}

	return nil
}

func (a *UserAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if ok && a.VerifyCredentials(user, password) == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Make browser prompt for basic authentication
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}
