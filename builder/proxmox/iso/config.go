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
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type Config struct {
	common.Config `mapstructure:",squash"`
	// No longer required when deprecated boot iso options are removed
	commonsteps.ISOConfig `mapstructure:",squash"`
	// DEPRECATED. Define Boot ISO config with the `boot_iso` block instead.
	// Path to the ISO file to boot from, expressed as a
	// proxmox datastore path, for example
	// `local:iso/Fedora-Server-dvd-x86_64-29-1.2.iso`.
	// Either `iso_file` OR `iso_url` must be specifed.
	ISOFile string `mapstructure:"iso_file"`
	// DEPRECATED. Define Boot ISO config with the `boot_iso` block instead.
	// Proxmox storage pool onto which to upload
	// the ISO file.
	ISOStoragePool string `mapstructure:"iso_storage_pool"`
	// DEPRECATED. Define Boot ISO config with the `boot_iso` block instead.
	// Download the ISO directly from the PVE node rather than through Packer.
	//
	// Defaults to `false`
	ISODownloadPVE bool `mapstructure:"iso_download_pve"`
	// DEPRECATED. Define Boot ISO config with the `boot_iso` block instead.
	// If true, remove the mounted ISO from the template
	// after finishing. Defaults to `false`.
	UnmountISO bool `mapstructure:"unmount_iso"`
	// Boot ISO attached to the virtual machine.
	//
	// JSON Example:
	//
	// ```json
	//
	//	"boot_iso": {
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
	//	boot_iso {
	//	  type = "scsi"
	//	  iso_file = "local:iso/debian-12.5.0-amd64-netinst.iso"
	//	  unmount = true
	//	  iso_checksum = "sha512:33c08e56c83d13007e4a5511b9bf2c4926c4aa12fd5dd56d493c0653aecbab380988c5bf1671dbaea75c582827797d98c4a611f7fb2b131fbde2c677d5258ec9"
	//	}
	//
	// ```
	// See [ISOs](#isos) for additional options.
	BootISO common.ISOsConfig `mapstructure:"boot_iso" required:"true"`
}

func (c *Config) Prepare(raws ...interface{}) ([]string, []string, error) {
	var errs *packersdk.MultiError
	_, warnings, merrs := c.Config.Prepare(c, raws...)
	if merrs != nil {
		errs = packersdk.MultiErrorAppend(errs, merrs)
	}

	// Convert deprecated config options
	if c.ISOFile != "" {
		warnings = append(warnings, "'iso_file' is deprecated and will be removed in a future release, define the boot iso options in a 'boot_iso' block")
		// Convert this field across to c.BootISO struct
		c.BootISO.ISOFile = c.ISOFile
	}
	if c.ISOStoragePool != "" {
		warnings = append(warnings, "'iso_storage_pool' is deprecated and will be removed in a future release, define the boot iso options in a 'boot_iso' block")
		c.BootISO.ISOStoragePool = c.ISOStoragePool
	}
	if c.ISODownloadPVE {
		warnings = append(warnings, "'iso_download_pve' is deprecated and will be removed in a future release, define the boot iso options in a 'boot_iso' block")
		c.BootISO.ISODownloadPVE = c.ISODownloadPVE
	}
	if len(c.ISOUrls) > 0 {
		warnings = append(warnings, "'iso_urls' is deprecated and will be removed in a future release, define the boot iso options in a 'boot_iso' block")
		c.BootISO.ISOUrls = c.ISOUrls
	}
	if c.RawSingleISOUrl != "" {
		warnings = append(warnings, "'iso_url' is deprecated and will be removed in a future release, define the boot iso options in a 'boot_iso' block")
		c.BootISO.RawSingleISOUrl = c.RawSingleISOUrl
	}
	if c.ISOChecksum != "" {
		warnings = append(warnings, "'iso_checksum' is deprecated and will be removed in a future release, define the boot iso options in a 'boot_iso' block")
		c.BootISO.ISOChecksum = c.ISOChecksum
	}
	if c.UnmountISO {
		warnings = append(warnings, "'unmount_iso' is deprecated and will be removed in a future release, define the boot iso options in a 'boot_iso' block")
		c.BootISO.Unmount = c.UnmountISO
	}
	if c.TargetPath != "" {
		warnings = append(warnings, "'iso_target_path' is deprecated and will be removed in a future release, define the boot iso options in a 'boot_iso' block")
		c.BootISO.TargetPath = c.TargetPath
	}
	if c.TargetExtension != "" {
		warnings = append(warnings, "'iso_target_extension' is deprecated and will be removed in a future release, define the boot iso options in a 'boot_iso' block")
		c.BootISO.TargetExtension = c.TargetExtension
	}

	// Check Boot ISO config
	// Either a pre-uploaded ISO should be referenced in iso_file, OR a URL
	// (possibly to a local file) to an ISO file that will be downloaded and
	// then uploaded to Proxmox.
	if c.BootISO.ISOFile != "" {
		c.BootISO.ShouldUploadISO = false
	} else {
		c.BootISO.DownloadPathKey = "downloaded_iso_path"
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
	// For backwards compatibility <= v1.8, set ide2 as default if not configured
	switch c.BootISO.Type {
	case "ide", "sata", "scsi":
	case "":
		log.Print("boot_iso device type not set, using default type 'ide' and index '2'")
		c.BootISO.Type = "ide"
		c.BootISO.Index = "2"
	default:
		errs = packersdk.MultiErrorAppend(errs, errors.New("ISOs must be of type ide, sata or scsi. VirtIO not supported by Proxmox for ISO devices"))
	}
	if len(c.BootISO.CDFiles) > 0 || len(c.BootISO.CDContent) > 0 {
		if c.BootISO.ISOStoragePool == "" {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("boot_iso storage_pool not set for storage of generated ISO from cd_files or cd_content"))
		}
	}
	if len(c.BootISO.ISOUrls) != 0 && c.BootISO.ISOStoragePool == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("when specifying iso_url in a boot_iso block, iso_storage_pool must also be specified"))
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
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("one of iso_file, iso_url, or a combination of cd_files and cd_content must be specified for boot_iso"))
	}
	if len(c.BootISO.ISOConfig.ISOUrls) == 0 && c.BootISO.ISOConfig.RawSingleISOUrl == "" && c.BootISO.ISODownloadPVE {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("iso_download_pve can only be used together with iso_url"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return nil, warnings, errs
	}
	return nil, warnings, nil
}
