// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,nicConfig,diskConfig,vgaConfig,additionalISOsConfig

package proxmoxiso

import (
	"errors"

	proxmoxcommon "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type Config struct {
	proxmoxcommon.Config `mapstructure:",squash"`

	commonsteps.ISOConfig `mapstructure:",squash"`
	// Path to the ISO file to boot from, expressed as a
	// proxmox datastore path, for example
	// `local:iso/Fedora-Server-dvd-x86_64-29-1.2.iso`.
	// Either `iso_file` OR `iso_url` must be specifed.
	ISOFile string `mapstructure:"iso_file"`
	// Proxmox storage pool onto which to upload
	// the ISO file.
	ISOStoragePool string `mapstructure:"iso_storage_pool"`
	// Download the specified `iso_url` directly from
	// the PVE node. Defaults to `false`.
	// By default Packer downloads the ISO and uploads it in a second step, this
	// option lets Proxmox handle downloading the ISO directly from the server.
	ISODownloadPVE bool `mapstructure:"iso_download_pve"`
	// If true, remove the mounted ISO from the template
	// after finishing. Defaults to `false`.
	UnmountISO      bool `mapstructure:"unmount_iso"`
	shouldUploadISO bool
}

func (c *Config) Prepare(raws ...interface{}) ([]string, []string, error) {
	var errs *packersdk.MultiError
	_, warnings, merrs := c.Config.Prepare(c, raws...)
	if merrs != nil {
		errs = packersdk.MultiErrorAppend(errs, merrs)
	}

	// Check ISO config
	// Either a pre-uploaded ISO should be referenced in iso_file, OR a URL
	// (possibly to a local file) to an ISO file that will be downloaded and
	// then uploaded to Proxmox.
	// If iso_download_pve is true, iso_url will be downloaded directly to the
	// PVE node.
	if c.ISOFile != "" {
		c.shouldUploadISO = false
	} else {
		isoWarnings, isoErrors := c.ISOConfig.Prepare(&c.Ctx)
		errs = packersdk.MultiErrorAppend(errs, isoErrors...)
		warnings = append(warnings, isoWarnings...)
		c.shouldUploadISO = true
	}

	if (c.ISOFile == "" && len(c.ISOConfig.ISOUrls) == 0) || (c.ISOFile != "" && len(c.ISOConfig.ISOUrls) != 0) {
		errs = packersdk.MultiErrorAppend(errs, errors.New("either iso_file or iso_url, but not both, must be specified"))
	}
	if len(c.ISOConfig.ISOUrls) != 0 && c.ISOStoragePool == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("when specifying iso_url, iso_storage_pool must also be specified"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return nil, warnings, errs
	}
	return nil, warnings, nil
}
