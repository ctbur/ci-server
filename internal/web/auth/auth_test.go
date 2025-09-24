package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const htpasswd = `
test1:$2y$05$vyEpG2uWhCB36knMvzIDc.k43J8Hyx84gMwlDKpcWsGH/Qi9QjrXe

test2:$2y$05$LstBCg/Z9DRNeFae8wq/duuWAzk5JFbB8kTIptHITu.j0iGXmCqZu
`

func GetTestAuth(t *testing.T) UserAuth {
	a, err := FromHtpasswd(htpasswd)
	if err != nil {
		t.Error(err)
	}

	return a
}

func TestUserDoesNotExist(t *testing.T) {
	a := GetTestAuth(t)

	err := a.VerifyCredentials("not_a_user", "1234")
	if !errors.Is(err, ErrUserDoesNotExist) {
		t.Fail()
	}
}

func TestPasswordMismatch(t *testing.T) {
	a := GetTestAuth(t)

	err := a.VerifyCredentials("test1", "4321")
	if !errors.Is(err, ErrPasswordMismatch) {
		t.Fail()
	}

	err = a.VerifyCredentials("test2", "asdf")
	if !errors.Is(err, ErrPasswordMismatch) {
		t.Fail()
	}
}

func TestPasswordMatch(t *testing.T) {
	a := GetTestAuth(t)

	err := a.VerifyCredentials("test1", "1234")
	if err != nil {
		t.Fail()
	}

	err = a.VerifyCredentials("test2", "4321")
	if err != nil {
		t.Fail()
	}
}

func dummyHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Success"))
}

func TestMiddlewareUnauthorized(t *testing.T) {
	userAuth := GetTestAuth(t)
	handler := Middleware(userAuth, http.HandlerFunc(dummyHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("test1", "invalid_password")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusUnauthorized)
	}

	authHeader := rr.Header().Get("WWW-Authenticate")
	expectedHeader := `Basic realm="restricted", charset="UTF-8"`
	if authHeader != expectedHeader {
		t.Errorf("handler returned wrong WWW-Authenticate header: got %v want %v", authHeader, expectedHeader)
	}
}

func TestMiddlewareNoAuth(t *testing.T) {
	userAuth := GetTestAuth(t)
	handler := Middleware(userAuth, http.HandlerFunc(dummyHandler))

	// No basic auth header
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusUnauthorized)
	}
}

func TestMiddlewareSuccess(t *testing.T) {
	userAuth := GetTestAuth(t)
	handler := Middleware(userAuth, http.HandlerFunc(dummyHandler))

	// Valid credentials
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("test1", "1234")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	expected := "Success"
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expected)
	}
}
