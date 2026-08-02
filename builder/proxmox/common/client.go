// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/Telmate/proxmox-api-go/proxmox"
)

func newProxmoxClient(config Config) (*proxmox.Client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.SkipCertValidation,
	}

	// Create an HTTP client that respects standard proxy environment variables
	// (HTTPS_PROXY, HTTP_PROXY, NO_PROXY). The upstream Telmate library sets
	// Proxy: nil when no explicit proxy string is provided, which disables
	// proxy support entirely. By passing our own http.Client, we ensure
	// proxy env vars are honored.
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:    tlsConfig,
			DisableCompression: true,
			Proxy:              http.ProxyFromEnvironment,
		},
	}

	client, err := proxmox.NewClient(strings.TrimSuffix(config.proxmoxURL.String(), "/"), httpClient, "", tlsConfig, "", int(config.TaskTimeout.Seconds()))
	if err != nil {
		return nil, wrapTLSError(err)
	}

	*proxmox.Debug = config.PackerDebug

	if config.Token != "" {
		// configure token auth
		log.Print("using token auth")
		client.SetAPIToken(config.Username, config.Token)
	} else {
		// fallback to login if not using tokens
		log.Print("using password auth")
		err = client.Login(config.Username, config.Password, "")
		if err != nil {
			return nil, wrapTLSError(err)
		}
	}

	return client, nil
}

// wrapTLSError inspects err for known x509 certificate errors and, if found,
// wraps it with an actionable hint pointing to insecure_skip_tls_verify.
// All other errors are returned unchanged.
func wrapTLSError(err error) error {
	if err == nil {
		return nil
	}

	var unknownAuthority x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var certInvalid x509.CertificateInvalidError

	if errors.As(err, &unknownAuthority) || errors.As(err, &hostnameErr) || errors.As(err, &certInvalid) {
		return fmt.Errorf(
			"TLS certificate verification failed connecting to Proxmox: %w\n\n"+
				"Hint: If your Proxmox server uses a self-signed certificate, add\n"+
				"  insecure_skip_tls_verify = true\n"+
				"to your builder configuration. Do NOT use this option in production.",
				err,
		)
	}

	return err
}
