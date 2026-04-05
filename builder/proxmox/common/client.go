// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"crypto/tls"
	"log"
	"net/http"
	"strings"

	"github.com/Telmate/proxmox-api-go/proxmox"
)

func newProxmoxClient(config Config) (*proxmox.Client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.SkipCertValidation,
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
			Proxy:           http.ProxyFromEnvironment,
		},
	}

	client, err := proxmox.NewClient(strings.TrimSuffix(config.proxmoxURL.String(), "/"), httpClient, "", tlsConfig, "", int(config.TaskTimeout.Seconds()))
	if err != nil {
		return nil, err
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
			return nil, err
		}
	}

	return client, nil
}
