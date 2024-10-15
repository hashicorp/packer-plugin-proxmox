// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc mapstructure-to-hcl2 -type Config,DatasourceOutput
package proxmoxtemplate

import (
	"errors"
	"fmt"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/hcl2helper"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/zclconf/go-cty/cty"
	"net/url"
	"os"
)

type Config struct {
	// URL to the Proxmox API, including the full path,
	// so `https://<server>:<port>/api2/json` for example.
	// Can also be set via the `PROXMOX_URL` environment variable.
	ProxmoxURLRaw string `mapstructure:"proxmox_url"`
	proxmoxURL    *url.URL
	// Skip validating the certificate.
	SkipCertValidation bool `mapstructure:"insecure_skip_tls_verify"`
	// Username when authenticating to Proxmox, including
	// the realm. For example `user@pve` to use the local Proxmox realm. When using
	// token authentication, the username must include the token id after an exclamation
	// mark. For example, `user@pve!tokenid`.
	// Can also be set via the `PROXMOX_USERNAME` environment variable.
	Username string `mapstructure:"username"`
	// Password for the user.
	// For API tokens please use `token`.
	// Can also be set via the `PROXMOX_PASSWORD` environment variable.
	// Either `password` or `token` must be specifed. If both are set,
	// `token` takes precedence.
	Password string `mapstructure:"password"`
	// Token for authenticating API calls.
	// This allows the API client to work with API tokens instead of user passwords.
	// Can also be set via the `PROXMOX_TOKEN` environment variable.
	// Either `password` or `token` must be specifed. If both are set,
	// `token` takes precedence.
	Token string `mapstructure:"token"`
}

type Datasource struct {
	config Config
}

type DatasourceOutput struct {
	Foo string `mapstructure:"foo"`
	Bar string `mapstructure:"bar"`
}

func (d *Datasource) ConfigSpec() hcldec.ObjectSpec {
	return d.config.FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Configure(raws ...interface{}) error {
	err := config.Decode(&d.config, nil, raws...)
	if err != nil {
		return err
	}

	var errs *packersdk.MultiError

	// Defaults
	if d.config.ProxmoxURLRaw == "" {
		d.config.ProxmoxURLRaw = os.Getenv("PROXMOX_URL")
	}
	if d.config.Username == "" {
		d.config.Username = os.Getenv("PROXMOX_USERNAME")
	}
	if d.config.Password == "" {
		d.config.Password = os.Getenv("PROXMOX_PASSWORD")
	}
	if d.config.Token == "" {
		d.config.Token = os.Getenv("PROXMOX_TOKEN")
	}

	// Required configurations that will display errors if not set
	if d.config.Username == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("username must be specified"))
	}
	if d.config.Password == "" && d.config.Token == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("password or token must be specified"))
	}
	if d.config.ProxmoxURLRaw == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("proxmox_url must be specified"))
	}
	if d.config.proxmoxURL, err = url.Parse(d.config.ProxmoxURLRaw); err != nil {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse proxmox_url: %s", err))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}
	return nil
}

func (d *Datasource) OutputSpec() hcldec.ObjectSpec {
	return (&DatasourceOutput{}).FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Execute() (cty.Value, error) {
	output := DatasourceOutput{
		Foo: "foo-value",
		Bar: "bar-value",
	}
	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}
