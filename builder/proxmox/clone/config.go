// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,cloudInitIpconfig

package proxmoxclone

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strings"

	proxmoxcommon "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

type Config struct {
	proxmoxcommon.Config `mapstructure:",squash"`

	// The name of the VM Packer should clone and build from.
	// Either `clone_vm` or `clone_vm_id` must be specifed.
	CloneVM string `mapstructure:"clone_vm" required:"true"`
	// The ID of the VM Packer should clone and build from.
	// Proxmox VMIDs are limited to the range 100-999999999.
	// Either `clone_vm` or `clone_vm_id` must be specifed.
	CloneVMID int `mapstructure:"clone_vm_id" required:"true"`
	// Whether to run a full or shallow clone from the base clone_vm. Defaults to `true`.
	FullClone config.Trilean `mapstructure:"full_clone" required:"false"`

	// Set nameserver IP address(es) via Cloud-Init.
	// If not given, the same setting as on the host is used.
	Nameserver string `mapstructure:"nameserver" required:"false"`
	// Set the DNS searchdomain via Cloud-Init.
	// If not given, the same setting as on the host is used.
	Searchdomain string `mapstructure:"searchdomain" required:"false"`
	// Set IP address and gateway via Cloud-Init.
	// See the [CloudInit Ip Configuration](#cloudinit-ip-configuration) documentation for fields.
	Ipconfigs []cloudInitIpconfig `mapstructure:"ipconfig" required:"false"`
}

// If you have configured more than one network interface, make sure to match the order of
// `network_adapters` and `ipconfig`.
//
// Usage example (JSON):
//
// ```json
// [
//
//	{
//	  "ip": "192.168.1.55/24",
//	  "gateway": "192.168.1.1",
//	  "ip6": "fda8:a260:6eda:20::4da/128",
//	  "gateway6": "fda8:a260:6eda:20::1"
//	}
//
// ]
// ```
type cloudInitIpconfig struct {
	// Either an IPv4 address (CIDR notation) or `dhcp`.
	Ip string `mapstructure:"ip" required:"false"`
	// IPv4 gateway.
	Gateway string `mapstructure:"gateway" required:"false"`
	// Can be an IPv6 address (CIDR notation), `auto` (enables SLAAC), or `dhcp`.
	Ip6 string `mapstructure:"ip6" required:"false"`
	// IPv6 gateway.
	Gateway6 string `mapstructure:"gateway6" required:"false"`
}

func (c *Config) Prepare(raws ...interface{}) ([]string, []string, error) {
	var errs *packersdk.MultiError
	_, warnings, merrs := c.Config.Prepare(c, raws...)
	if merrs != nil {
		errs = packersdk.MultiErrorAppend(errs, merrs)
	}

	if c.CloneVM == "" && c.CloneVMID == 0 {
		errs = packersdk.MultiErrorAppend(errs, errors.New("one of clone_vm or clone_vm_id must be specified"))
	}
	if c.CloneVM != "" && c.CloneVMID != 0 {
		errs = packersdk.MultiErrorAppend(errs, errors.New("clone_vm and clone_vm_id cannot both be specified"))
	}
	// Technically Proxmox VMIDs are unsigned 32bit integers, but are limited to
	// the range 100-999999999. Source:
	// https://pve-devel.pve.proxmox.narkive.com/Pa6mH1OP/avoiding-vmid-reuse#post8
	if c.CloneVMID != 0 && (c.CloneVMID < 100 || c.CloneVMID > 999999999) {
		errs = packersdk.MultiErrorAppend(errs, errors.New("clone_vm_id must be in range 100-999999999"))
	}

	// Check validity of given IP addresses
	if c.Nameserver != "" {
		for _, nameserver := range strings.Split(c.Nameserver, " ") {
			_, err := netip.ParseAddr(nameserver)
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse nameserver: %s", err))
			}
		}
	}
	for _, i := range c.Ipconfigs {
		if i.Ip != "" && i.Ip != "dhcp" {
			_, _, err := net.ParseCIDR(i.Ip)
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse ipconfig.ip: %s", err))
			}
		}
		if i.Gateway != "" {
			_, err := netip.ParseAddr(i.Gateway)
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse ipconfig.gateway: %s", err))
			}
		}
		if i.Ip6 != "" && i.Ip6 != "auto" && i.Ip6 != "dhcp" {
			_, _, err := net.ParseCIDR(i.Ip6)
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse ipconfig.ip6: %s", err))
			}
		}
		if i.Gateway6 != "" {
			_, err := netip.ParseAddr(i.Gateway6)
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse ipconfig.gateway6: %s", err))
			}
		}
	}
	if len(c.NICs) < len(c.Ipconfigs) {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("%d ipconfig blocks given, but only %d network interfaces defined", len(c.Ipconfigs), len(c.NICs)))
	}

	// each device type has a maximum number of devices that can be attached.
	// count disks, additional isos configured for each device type, error if too many.
	ideCount := 0
	sataCount := 0
	scsiCount := 0
	virtIOCount := 0
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
	// count additional_iso_files devices
	for _, iso := range c.AdditionalISOFiles {
		// get device type from iso.Device
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

// Convert Ipconfig attributes into a Proxmox-API compatible string
func (c cloudInitIpconfig) String() string {
	options := []string{}
	if c.Ip != "" {
		options = append(options, "ip="+c.Ip)
	}
	if c.Gateway != "" {
		options = append(options, "gw="+c.Gateway)
	}
	if c.Ip6 != "" {
		options = append(options, "ip6="+c.Ip6)
	}
	if c.Gateway6 != "" {
		options = append(options, "gw6="+c.Gateway6)
	}
	return strings.Join(options, ",")
}
