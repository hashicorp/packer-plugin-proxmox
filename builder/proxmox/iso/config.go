// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,nicConfig,diskConfig,vgaConfig,ISOsConfig

package proxmoxiso

import (
	"errors"
	"fmt"
	"log"

	common "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type Config struct {
	common.Config `mapstructure:",squash"`

	// Boot ISO attached to the virtual machine.
	//
	// JSON Example:
	//
	// ```json
	//
	//	"iso": {
	//			  "type": "scsi",
	//			  "iso_file": "local:iso/debian-12.5.0-amd64-netinst.iso",
	//			  "unmount": true,
	//			  "iso_checksum": "sha512:33c08e56c83d13007e4a5511b9bf2c4926c4aa12fd5dd56d493c0653aecbab380988c5bf1671dbaea75c582827797d98c4a611f7fb2b131fbde2c677d5258ec9"
	//		}
	//
	// ```
	// HCL2 example:
	//
	// ```hcl
	//
	//	iso {
	//	  type = "scsi"
	//	  iso_file = "local:iso/debian-12.5.0-amd64-netinst.iso"
	//	  unmount = true
	//	  iso_checksum = "sha512:33c08e56c83d13007e4a5511b9bf2c4926c4aa12fd5dd56d493c0653aecbab380988c5bf1671dbaea75c582827797d98c4a611f7fb2b131fbde2c677d5258ec9"
	//	}
	//
	// ```
	// See [ISOs](#isos) for additional options.
	BootISO common.ISOsConfig `mapstructure:"iso" required:"true"`
}

func (c *Config) Prepare(raws ...interface{}) ([]string, []string, error) {
	var errs *packersdk.MultiError
	_, warnings, merrs := c.Config.Prepare(c, raws...)
	if merrs != nil {
		errs = packersdk.MultiErrorAppend(errs, merrs)
	}

	// Check Boot ISO config
	// Either a pre-uploaded ISO should be referenced in iso_file, OR a URL
	// (possibly to a local file) to an ISO file that will be downloaded and
	// then uploaded to Proxmox.
	if c.BootISO.ISOFile != "" {
		c.BootISO.ShouldUploadISO = false
	} else {
		c.BootISO.DownloadPathKey = "downloaded_iso_path_0"
		if len(c.BootISO.CDFiles) > 0 || len(c.BootISO.CDContent) > 0 {
			cdErrors := c.BootISO.CDConfig.Prepare(&c.Ctx)
			errs = packersdk.MultiErrorAppend(errs, cdErrors...)
		} else {
			isoWarnings, isoErrors := c.BootISO.ISOConfig.Prepare(&c.Ctx)
			errs = packersdk.MultiErrorAppend(errs, isoErrors...)
			warnings = append(warnings, isoWarnings...)
		}
		c.BootISO.ShouldUploadISO = true
	}
	// validate device type, assign if unset
	switch c.BootISO.Type {
	case "ide", "sata", "scsi":
	case "":
		log.Print("iso device type not set, using default type 'ide'")
		c.BootISO.Type = "ide"
	default:
		errs = packersdk.MultiErrorAppend(errs, errors.New("ISOs must be of type ide, sata or scsi. VirtIO not supported by Proxmox for ISO devices"))
	}
	if len(c.BootISO.CDFiles) > 0 || len(c.BootISO.CDContent) > 0 {
		if c.BootISO.ISOStoragePool == "" {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("storage_pool not set for storage of generated ISO from cd_files or cd_content"))
		}
	}
	if len(c.BootISO.ISOUrls) != 0 && c.BootISO.ISOStoragePool == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("when specifying iso_url in an iso block, iso_storage_pool must also be specified"))
	}
	// Check only one option is present
	options := 0
	if c.BootISO.ISOFile != "" {
		options++
	}
	if len(c.BootISO.ISOConfig.ISOUrls) > 0 || c.BootISO.ISOConfig.RawSingleISOUrl != "" {
		options++
	}
	if len(c.BootISO.CDFiles) > 0 || len(c.BootISO.CDContent) > 0 {
		options++
	}
	if options != 1 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("one of iso_file, iso_url, or a combination of cd_files and cd_content must be specified for iso"))
	}
	if len(c.BootISO.ISOConfig.ISOUrls) == 0 && c.BootISO.ISOConfig.RawSingleISOUrl == "" && c.BootISO.ISODownloadPVE {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("iso_download_pve can only be used together with iso_url"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return nil, warnings, errs
	}
	return nil, warnings, nil
}
