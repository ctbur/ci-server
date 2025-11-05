package github

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/ctbur/ci-server/v2/internal/assert"
	"github.com/ctbur/ci-server/v2/internal/config"
)

func TestIssueJWT(t *testing.T) {
	testPrivateKey := `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDo3l2OsBs9l+K1
Wm+anfoETBD1t7vv31i7URPjLAbWrXfoRLvlABTCCIGaaaTItLqL6cJIpA+1vaqb
0G4Tz7WiDPJMJ80DQYN9VEfzLilNdQWG+Y72s7i2gkVcGrN8APwuAUPiGLI7lPXD
scjOkYpHOYqalAyHM0yjQD3WG7ZXYEo84MoMlPIhaJ5/Wx9k43Vm7KNl1mH9Z/l8
tdx0tQ5/97NVDz9QK7IZz80dZbY3gxEhg0hCjBpEkQrrIPONPkdNAIgEJDEjJlJD
8NwuVt9pIDz4u901MxWuVY99I8laYRi2YAPhkpI9qa5a/8BnxsMVvmTOdjPXfR+G
7AOYKb95AgMBAAECggEAANMlzW8z/3852TbXaZqo46pBzp7+mkpGGg6+1WmG4iyF
3dOdf0GFcUz+qYV5hRD3zq5Opvt2x0fjUm817pnIHMlzM8AZ1qq9HJznbtXxZUH2
eayJXDgVfcb/GCULkPM/cWnFe+fmvLWJu85Vxd6H2Pn8EtFWrIVq3nKoLWmWdx+B
VNB5O9bnx3YO3uz2Z3x4avqUeBvbuUbwaYeqNF7R2a/WYkqOGGuUW9UHlwks9l0Y
4PZ6X60sbQhKcIFE16Ol4FW5qKujbmBsK0qS4m161q2i59CYQF8C7WCOJSSfPF8W
e/hh8+qF4EXYwY7BDPPE4A/vwmZCt7qWa3Rg32K8UQKBgQD75Pf8lUBY3+XN1zFx
s/nzm1iSvYB2cl1X7Zwe4c9IJL9dj2MUihcAvNnTbQWIjLgR1sI7ympMg3pXqdPp
XBfAPP14zg9qiDDODGsabHqXMDi2Rab3MRwlT8lCbB19enpp9ukoi0u4Y5Pd6XOl
6bldJn3wFsPNdSfFsPA3QIy7kQKBgQDsqgLxPUb1Nzags2ALNujJH4kYCA+Fp8/V
ShZPqcFGawEdPzEMVawqXGUJMc5T8rBC6dxjAsOI394SdrKIbqUzZwaDQ4n61h+D
J2au3ovkyoI/NKqs+x1dZiZDMqSNOdcgRx/Q8asGviFcv9az1ld6zJTiFLpnf6LM
5lXfE0FBaQKBgQDjPPICaNp90q5LfaBbRNNuUmwbJN1o/U777Zzztx35pT0FuD7X
3qNVxQh01Vsyjk/Xt/fNXJN8pveNceV8FdpPUDYR70K1BluQ5l8QnWASWCwxMrCn
OyR6/HlBdKs98WnRgi9gphkPJLXWca4ktK7GO91M5ByLku7oRvDNx2uuIQKBgBUG
LWi85tbV5tZz2O5mHFvxnz4xSR+4frAV+tFs5SyaSOkOOg88dST2PEuKzyeKAbqQ
B/ILxs8cBCBjxwxzt91PI7b5gwJzjy0ZjPev8YGLs/JlfVwMmtk3P+LsVs3s+310
lBD9xxG8Rj51FF+5hN/12KwU51JWdmH5fFtq3HsxAoGBAN3EZkH6h6qSzIPe+nML
MSVkZBoGeoBSEF68lI5Tjid3966a7REwp4hckzWU14Dwx2qBEPLVHNe8m060xAq7
Sb4IRk/7baQtAAGhkTNe3gYQwTxlmuHT3HuNhhxTleED1vh0dmWFqJKmUQG838pG
0GDeVGyi0RXZThNyfdCTmmel
-----END PRIVATE KEY-----`

	rsaKey, err := config.LoadRSAPrivateKey(bytes.NewReader([]byte(testPrivateKey)))
	assert.NoError(t, err, "Failed to load RSA private key")

	gh := &GitHubApp{
		client:     http.DefaultClient,
		appID:      123456,
		privateKey: rsaKey,
	}

	jwt, err := gh.issueJWT(time.Unix(1762198371, 0))
	if err != nil {
		t.Fatalf("failed to issue JWT: %v", err)
	}

	expectedJWT := "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.eyJpYXQiOjE3NjIxOTgzMTEsImV4cCI6MTc2MjE5ODkxMSwiaXNzIjoxMjM0" +
		"NTZ9.DI4kuZ7UkmR2f7WevXXoTEG7Rm7hG29sItlWrMwcccOMSNgD3cWVVuegUqcdzlEPTv9qpTEWuv7u9HFrv6mhP8FQuwwB6ry4wxB5JuC" +
		"sSDo68RR9L3wdv2j6jrXARe-efLVyG_hrb4aP4WuvbcxA8TXm9Z70-Gv_0nR58MHKwDBgH6GjtWeeyYZvIp1uGYob9PNjkeQtQM3guix0E5M" +
		"WpHqJMtV7ZSQ3WvpuNbRZ_-cCi92pFDI-6zqhQb7EjIy9fraZkUCPyGwYpxJ1pWcS1ucWUJbLC99np6MpQ64ygLrlNwdlZPqzvfNRZXVNOdK" +
		"o4MJ2Cj6qUTg_PPIB8XpZbQ"
	assert.Equal(t, jwt, expectedJWT, "Incorrect JWT")
}
