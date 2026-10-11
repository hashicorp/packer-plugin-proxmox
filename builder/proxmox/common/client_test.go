// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"crypto/x509"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/stretchr/testify/require"
)

func TestTokenAuth(t *testing.T) {
	mockAPI := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "PVEAPIToken=dummy@vmhost!test-token=ac5293bf-15e2-477f-b04c-a6dfa7a46b80" {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
	}))
	defer mockAPI.Close()

	pmURL, _ := url.Parse(mockAPI.URL)
	config := Config{
		proxmoxURL:         pmURL,
		SkipCertValidation: false,
		Username:           "dummy@vmhost!test-token",
		Password:           "not-used",
		Token:              "ac5293bf-15e2-477f-b04c-a6dfa7a46b80",
	}

	client, err := newProxmoxClient(config)
	require.NoError(t, err)

	ref := proxmox.NewVmRef(110)
	ref.SetNode("node1")
	ref.SetVmType("qemu")
	err = client.Sendkey(ref, "ping")
	require.NoError(t, err)
}

func TestLogin(t *testing.T) {
	mockAPI := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// mock ticketing api
		if req.Method == http.MethodPost && req.URL.Path == "/access/ticket" {
			body, _ := ioutil.ReadAll(req.Body)
			values, _ := url.ParseQuery(string(body))
			user := values.Get("username")
			pass := values.Get("password")
			if user != "dummy@vmhost" || pass != "correct-horse-battery-staple" {
				rw.WriteHeader(http.StatusUnauthorized)
				return
			}

			_ = json.NewEncoder(rw).Encode(map[string]interface{}{
				"data": map[string]string{
					"username":            user,
					"ticket":              "dummy-ticket",
					"CSRFPreventionToken": "random-token",
				},
			})
			return
		}

		// validate ticket
		if req.Header["Authorization"][0] != "PVEAuthCookie=dummy-ticket" {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
	}))
	defer mockAPI.Close()

	pmURL, _ := url.Parse(mockAPI.URL)
	config := Config{
		proxmoxURL:         pmURL,
		SkipCertValidation: false,
		Username:           "dummy@vmhost",
		Password:           "correct-horse-battery-staple",
		Token:              "",
	}

	client, err := newProxmoxClient(config)
	require.NoError(t, err)

	ref := proxmox.NewVmRef(110)
	ref.SetNode("node1")
	ref.SetVmType("qemu")
	err = client.Sendkey(ref, "ping")
	require.NoError(t, err)
}

// TestWrapTLSError_UnknownAuthority verifies that an x509.UnknownAuthorityError
// is wrapped with a hint containing "insecure_skip_tls_verify".
func TestWrapTLSError_UnknownAuthority(t *testing.T) {
	orig := x509.UnknownAuthorityError{}
	wrapped := wrapTLSError(orig)
	require.Error(t, wrapped)
	require.Contains(t, wrapped.Error(), "insecure_skip_tls_verify",
		"expected hint about insecure_skip_tls_verify in TLS error")
}

// TestWrapTLSError_NonTLS verifies that non-TLS errors are returned unchanged.
func TestWrapTLSError_NonTLS(t *testing.T) {
	orig := errors.New("some unrelated error")
	result := wrapTLSError(orig)
	require.Equal(t, orig, result, "non-TLS error should pass through unchanged")
}

// TestWrapTLSError_Nil verifies that nil is returned unchanged.
func TestWrapTLSError_Nil(t *testing.T) {
	require.NoError(t, wrapTLSError(nil))
}

// TestNewProxmoxClient_TLSHint_PasswordAuth verifies that when the Proxmox API
// is served over TLS with an untrusted certificate and password auth is used,
// the returned error contains the insecure_skip_tls_verify hint.
func TestNewProxmoxClient_TLSHint_PasswordAuth(t *testing.T) {
	// httptest.NewTLSServer uses a self-signed certificate not trusted by Go's
	// default cert pool, so the TLS handshake will fail with an
	// x509.UnknownAuthorityError.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {}))
	defer srv.Close()

	pmURL, _ := url.Parse(srv.URL)
	config := Config{
		proxmoxURL:         pmURL,
		SkipCertValidation: false,
		Username:           "user@pam",
		Password:           "secret",
		Token:              "",
	}

	_, err := newProxmoxClient(config)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insecure_skip_tls_verify",
		"password-auth path should surface the insecure_skip_tls_verify hint on TLS failure")
}

// TestNewProxmoxClient_TLSHint_TokenAuth verifies that when the Proxmox API
// is served over TLS with an untrusted certificate and token auth is used,
// the returned error contains the insecure_skip_tls_verify hint.
// proxmox.NewClient performs an initial connection check, so the x509 error
// is surfaced before SetAPIToken is ever called.
func TestNewProxmoxClient_TLSHint_TokenAuth(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {}))
	defer srv.Close()

	pmURL, _ := url.Parse(srv.URL)
	config := Config{
		proxmoxURL:         pmURL,
		SkipCertValidation: false,
		Username:           "user@pam!mytoken",
		Password:           "",
		Token:              "abc123",
	}

	_, err := newProxmoxClient(config)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insecure_skip_tls_verify",
		"token-auth path should surface the insecure_skip_tls_verify hint on TLS failure")
}
