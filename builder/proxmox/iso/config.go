// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,nicConfig,diskConfig,vgaConfig,ISOsConfig

package proxmoxiso

import (
	"errors"

	proxmoxcommon "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type Config struct {
	proxmoxcommon.Config `mapstructure:",squash"`
}

func (c *Config) Prepare(raws ...interface{}) ([]string, []string, error) {
	var errs *packersdk.MultiError
	_, warnings, merrs := c.Config.Prepare(c, raws...)
	if merrs != nil {
		errs = packersdk.MultiErrorAppend(errs, merrs)
	}

	if len(c.ISOs) < 1 {
		errs = packersdk.MultiErrorAppend(errs, errors.New("at least one ISO device is required"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return nil, warnings, errs
	}
	return nil, warnings, nil
}
