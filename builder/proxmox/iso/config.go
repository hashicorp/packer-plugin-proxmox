// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,nicConfig,diskConfig,vgaConfig,additionalISOsConfig

package proxmoxiso

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"

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
	// Bus type and bus index that the ISO will be mounted on. Can be `ideX`,
	// `sataX` or `scsiX`.
	// For `ide` the bus index ranges from 0 to 3, for `sata` from 0 to 5 and for
	// `scsi` from 0 to 30.
	// Defaults to `ide2`
	ISODevice string `mapstructure:"iso_device"`
	// Proxmox storage pool onto which to upload
	// the ISO file.
	ISOStoragePool string `mapstructure:"iso_storage_pool"`
	// Download the ISO directly from the PVE node rather than through Packer.
	//
	// Defaults to `false`
	ISODownloadPVE bool `mapstructure:"iso_download_pve"`
	// If true, remove the mounted ISO from the template
	// after finishing. Defaults to `false`.
	UnmountISO bool `mapstructure:"unmount_iso"`
	// Keep CDRom device attached to template if unmounting ISO. Defaults to `false`.
	// Has no effect if unmount is `false`
	UnmountKeepDevice bool `mapstructure:"unmount_keep_device"`
	shouldUploadISO   bool
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

	// each device type has a maximum number of devices that can be attached.
	// count iso, disks, additional isos configured for each device type, error if too many.
	ideCount := 0
	sataCount := 0
	scsiCount := 0
	virtIOCount := 0

	if c.ISODevice == "" {
		// set default ISO boot device ide2 if no value specified
		log.Printf("iso_device not set, using default 'ide2'")
		c.ISODevice = "ide2"
		ideCount++
		// Pass c.ISODevice value over to common config to ensure device index not used by disks
		c.ISOBuilderCDROMDevice = "ide2"
	} else {
		// Pass c.ISODevice value over to common config to ensure device index not used by disks
		c.ISOBuilderCDROMDevice = c.ISODevice
		// get device from ISODevice config
		rd := regexp.MustCompile(`\D+`)
		device := rd.FindString(c.ISODevice)
		// get index from ISODevice config
		rb := regexp.MustCompile(`\d+`)
		_, err := strconv.Atoi(rb.FindString(c.ISODevice))
		if err != nil {
			errs = packersdk.MultiErrorAppend(errs, errors.New("iso_device value doesn't contain a valid index number. Expected format is <device type><index number>, eg. scsi0. received value: "+c.ISODevice))
		}
		// count iso
		switch device {
		case "ide":
			ideCount++
		case "sata":
			sataCount++
		case "scsi":
			scsiCount++
		}
		for idx, iso := range c.AdditionalISOFiles {
			if iso.Device == c.ISODevice {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("conflicting device assignment between iso_device (%s) and additional_iso_files block %d device", c.ISODevice, idx+1))
			}
			// count additional isos
			// get device from iso.Device
			rd := regexp.MustCompile(`\D+`)
			device := rd.FindString(iso.Device)
			switch device {
			case "ide":
				ideCount++
			case "sata":
				sataCount++
			case "scsi":
				scsiCount++
			}
		}
		if !(device == "ide" || device == "sata" || device == "scsi") {
			errs = packersdk.MultiErrorAppend(errs, errors.New("iso_device must be of type ide, sata or scsi. VirtIO not supported for ISO devices"))
		}
	}

	// count disks
	for _, disks := range c.Disks {
		switch disks.Type {
		case "ide":
			ideCount++
		case "sata":
			sataCount++
		case "scsi":
			scsiCount++
		case "virtio":
			virtIOCount++
		}
	}
	// validate device type allocations
	if ideCount > 4 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("maximum 4 IDE disks and ISOs supported"))
	}
	if sataCount > 6 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("maximum 6 SATA disks and ISOs supported"))
	}
	if scsiCount > 31 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("maximum 31 SCSI disks and ISOs supported"))
	}
	if virtIOCount > 16 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("maximum 16 VirtIO disks supported"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return nil, warnings, errs
	}
	return nil, warnings, nil
}
