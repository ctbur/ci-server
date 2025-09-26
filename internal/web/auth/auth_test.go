package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ctbur/ci-server/v2/internal/assert"
)

const htpasswd = `
test1:$2y$05$vyEpG2uWhCB36knMvzIDc.k43J8Hyx84gMwlDKpcWsGH/Qi9QjrXe

test2:$2y$05$LstBCg/Z9DRNeFae8wq/duuWAzk5JFbB8kTIptHITu.j0iGXmCqZu
`

func TestVerifyCredentials(t *testing.T) {
	userAuth, err := FromHtpasswd(htpasswd)
	if err != nil {
		t.Error(err)
	}

	testCases := []struct {
		desc      string
		username  string
		password  string
		wantError error
	}{
		{
			desc:      "user does not exist",
			username:  "not_a_user",
			password:  "1234",
			wantError: ErrUserDoesNotExist,
		},
		{
			desc:      "password mismatch for test1",
			username:  "test1",
			password:  "4321",
			wantError: ErrPasswordMismatch,
		},
		{
			desc:      "password mismatch for test2",
			username:  "test2",
			password:  "asdf",
			wantError: ErrPasswordMismatch,
		},
		{
			desc:      "password match for test1",
			username:  "test1",
			password:  "1234",
			wantError: nil,
		},
		{
			desc:      "password match for test2",
			username:  "test2",
			password:  "4321",
			wantError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			gotError := userAuth.VerifyCredentials(tc.username, tc.password)
			if !errors.Is(gotError, tc.wantError) {
				t.Errorf("got %v, want %v", gotError, tc.wantError)
			}
		})
	}
}

type BasicAuth struct {
	User, Password string
}

func TestMiddleware(t *testing.T) {
	testCases := []struct {
		desc            string
		auth            *BasicAuth
		wantHTTPCode    int
		wwwAuthenticate string
	}{
		{
			desc:            "no auth header",
			auth:            nil,
			wantHTTPCode:    http.StatusUnauthorized,
			wwwAuthenticate: `Basic realm="restricted", charset="UTF-8"`,
		},
		{
			desc: "invalid credentials",
			auth: &BasicAuth{
				User:     "test1",
				Password: "wrong_password",
			},
			wantHTTPCode:    http.StatusUnauthorized,
			wwwAuthenticate: `Basic realm="restricted", charset="UTF-8"`,
		},
		{
			desc: "valid credentials",
			auth: &BasicAuth{
				User:     "test1",
				Password: "1234",
			},
			wantHTTPCode:    http.StatusOK,
			wwwAuthenticate: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Create handler with auth middleware
			userAuth, err := FromHtpasswd(htpasswd)
			if err != nil {
				t.Error(err)
			}
			handler := userAuth.Middleware(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("Success"))
				},
			))

			// Create request with given basic auth header
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.auth != nil {
				req.SetBasicAuth(tc.auth.User, tc.auth.Password)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, rr.Code, tc.wantHTTPCode, "handler returned wrong status code")
			assert.Equal(
				t, rr.Header().Get("WWW-Authenticate"), tc.wwwAuthenticate,
				"handler returned wrong WWW-Authenticate header",
			)
		})
	}
}
