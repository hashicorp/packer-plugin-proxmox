// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"strings"

	"github.com/Telmate/proxmox-api-go/proxmox"
)

func newProxmoxClient(ctx context.Context, config Config) (*proxmox.Client, error) {
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

	var debugLogger *logger
	if config.PackerDebug {
		debugLogger = &logger{}
	}

	client, err := proxmox.NewClient(strings.TrimSuffix(config.proxmoxURL.String(), "/"), httpClient, "", tlsConfig, "", int(config.TaskTimeout.Seconds()), debugLogger)
	if err != nil {
		return nil, err
	}

	if config.Token != "" {
		// configure token auth
		log.Print("using token auth")
		var token proxmox.ApiToken
		if err = token.Parse(config.Username + "!" + config.Token); err != nil {
			return nil, err
		}
		client.SetAPIToken(token)
	} else {
		// fallback to login if not using tokens
		log.Print("using password auth")
		err = client.Login(ctx, config.Username, config.Password, "")
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}

type logger struct{}

func (*logger) Write(data []byte) (n int, err error) {
	log.Println(string(data))
	return len(data), nil
}
