// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"crypto/tls"
	"log"
	"strings"

	"github.com/Telmate/proxmox-api-go/proxmox"
)

func newProxmoxClient(ctx context.Context, config Config) (*proxmox.Client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.SkipCertValidation,
	}

	client, err := proxmox.NewClient(strings.TrimSuffix(config.proxmoxURL.String(), "/"), nil, "", tlsConfig, "", int(config.TaskTimeout.Seconds()), config.PackerDebug)
	if err != nil {
		return nil, err
	}

	if config.Token != "" {
		log.Print("using token auth")
		var tokenID proxmox.ApiTokenID
		if err := tokenID.Parse(config.Username); err != nil {
			return nil, err
		}
		client.SetAPIToken(tokenID, proxmox.ApiTokenSecret(config.Token))
	} else {
		log.Print("using password auth")
		err = client.Login(ctx, config.Username, config.Password, "")
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}
