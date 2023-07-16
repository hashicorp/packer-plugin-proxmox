// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,NICConfig,diskConfig,rng0Config,pciDeviceConfig,vgaConfig,additionalISOsConfig,efiConfig

package proxmox

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/bootcommand"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
	"github.com/mitchellh/mapstructure"
)

type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	commonsteps.HTTPConfig `mapstructure:",squash"`
	bootcommand.BootConfig `mapstructure:",squash"`
	BootKeyInterval        time.Duration       `mapstructure:"boot_key_interval"`
	Comm                   communicator.Config `mapstructure:",squash"`

	ProxmoxURLRaw      string `mapstructure:"proxmox_url"`
	proxmoxURL         *url.URL
	SkipCertValidation bool          `mapstructure:"insecure_skip_tls_verify"`
	Username           string        `mapstructure:"username"`
	Password           string        `mapstructure:"password"`
	Token              string        `mapstructure:"token"`
	Node               string        `mapstructure:"node"`
	Pool               string        `mapstructure:"pool"`
	TaskTimeout        time.Duration `mapstructure:"task_timeout"`

	VMName string `mapstructure:"vm_name"`
	VMID   int    `mapstructure:"vm_id"`

	Boot           string            `mapstructure:"boot"`
	Memory         int               `mapstructure:"memory"`
	BalloonMinimum int               `mapstructure:"ballooning_minimum"`
	Cores          int               `mapstructure:"cores"`
	CPUType        string            `mapstructure:"cpu_type"`
	Sockets        int               `mapstructure:"sockets"`
	Numa           bool              `mapstructure:"numa"`
	OS             string            `mapstructure:"os"`
	BIOS           string            `mapstructure:"bios"`
	EFIConfig      efiConfig         `mapstructure:"efi_config"`
	EFIDisk        string            `mapstructure:"efidisk"`
	Machine        string            `mapstructure:"machine"`
	Rng0           rng0Config        `mapstructure:"rng0"`
	VGA            vgaConfig         `mapstructure:"vga"`
	NICs           []NICConfig       `mapstructure:"network_adapters"`
	Disks          []diskConfig      `mapstructure:"disks"`
	PCIDevices     []pciDeviceConfig `mapstructure:"pci_devices"`
	Serials        []string          `mapstructure:"serials"`
	Agent          config.Trilean    `mapstructure:"qemu_agent"`
	SCSIController string            `mapstructure:"scsi_controller"`
	Onboot         bool              `mapstructure:"onboot"`
	DisableKVM     bool              `mapstructure:"disable_kvm"`

	TemplateName        string `mapstructure:"template_name"`
	TemplateDescription string `mapstructure:"template_description"`

	CloudInit            bool   `mapstructure:"cloud_init"`
	CloudInitStoragePool string `mapstructure:"cloud_init_storage_pool"`

	AdditionalISOFiles []additionalISOsConfig `mapstructure:"additional_iso_files"`
	VMInterface        string                 `mapstructure:"vm_interface"`

	Ctx interpolate.Context `mapstructure-to-hcl2:",skip"`
}

type additionalISOsConfig struct {
	commonsteps.ISOConfig `mapstructure:",squash"`
	Device                string `mapstructure:"device"`
	ISOFile               string `mapstructure:"iso_file"`
	ISOStoragePool        string `mapstructure:"iso_storage_pool"`
	Unmount               bool   `mapstructure:"unmount"`
	ShouldUploadISO       bool   `mapstructure-to-hcl2:",skip"`
	DownloadPathKey       string `mapstructure-to-hcl2:",skip"`
	commonsteps.CDConfig  `mapstructure:",squash"`
}

// - `network_adapters` (array of objects) - Network adapters attached to the
//   virtual machine. Example:
//
//   ```json
//   [
//     {
//       "model": "virtio",
//       "bridge": "vmbr0",
//       "vlan_tag": "10",
//       "firewall": true
//     }
//   ]
//   ```
type NICConfig struct {
	// Model of the virtual network adapter. Can be
	// `rtl8139`, `ne2k_pci`, `e1000`, `pcnet`, `virtio`, `ne2k_isa`,
	// `i82551`, `i82557b`, `i82559er`, `vmxnet3`, `e1000-82540em`,
	// `e1000-82544gc` or `e1000-82545em`. Defaults to `e1000`.
	Model string `mapstructure:"model"`
	// Number of packet queues to be used on the device.
	// Values greater than 1 indicate that the multiqueue feature is activated.
	// For best performance, set this to the number of cores available to the
	// virtual machine. CPU load on the host and guest systems will increase as
	// the traffic increases, so activate this option only when the VM has to
	// handle a great number of incoming connections, such as when the VM is
	// operating as a router, reverse proxy or a busy HTTP server. Requires
	// `virtio` network adapter. Defaults to `0`.
	PacketQueues int `mapstructure:"packet_queues"`
	// Give the adapter a specific MAC address. If
	// not set, defaults to a random MAC. If value is "repeatable", value of MAC
	// address is deterministic based on VM ID and NIC ID.
	MACAddress string `mapstructure:"mac_address"`
	// Set the maximum transmission unit for the adapter. Valid
	// range: 0 - 65520. If set to `1`, the MTU is inherited from the bridge
	// the adapter is attached to. Defaults to `0` (use Proxmox default).
	MTU int `mapstructure:"mtu"`
	// Required. Which Proxmox bridge to attach the
	// adapter to.
	Bridge string `mapstructure:"bridge"`
	// If the adapter should tag packets. Defaults to
	// no tagging.
	VLANTag string `mapstructure:"vlan_tag"`
	// If the interface should be protected by the firewall.
	// Defaults to `false`.
	Firewall bool `mapstructure:"firewall"`
}

// - `disks` (array of objects) - Disks attached to the virtual machine.
//   Example:
//
//   ```json
//   [
//     {
//       "type": "scsi",
//       "disk_size": "5G",
//       "storage_pool": "local-lvm",
//       "storage_pool_type": "lvm"
//     }
//   ]
//  ```
type diskConfig struct {
	// The type of disk. Can be `scsi`, `sata`, `virtio` or
	// `ide`. Defaults to `scsi`.
	Type string `mapstructure:"type"`
	// Required. Name of the Proxmox storage pool
	// to store the virtual machine disk on. A `local-lvm` pool is allocated
	// by the installer, for example.
	StoragePool string `mapstructure:"storage_pool"`
	// This option is deprecated.
	StoragePoolType string `mapstructure:"storage_pool_type"`
	// The size of the disk, including a unit suffix, such
	// as `10G` to indicate 10 gigabytes.
	Size string `mapstructure:"disk_size"`
	// How to cache operations to the disk. Can be
	// `none`, `writethrough`, `writeback`, `unsafe` or `directsync`.
	// Defaults to `none`.
	CacheMode string `mapstructure:"cache_mode"`
	// The format of the file backing the disk. Can be
	// `raw`, `cow`, `qcow`, `qed`, `qcow2`, `vmdk` or `cloop`. Defaults to
	// `raw`.
	DiskFormat string `mapstructure:"format"`
	// Create one I/O thread per storage controller, rather
	// than a single thread for all I/O. This can increase performance when
	// multiple disks are used. Requires `virtio-scsi-single` controller and a
	// `scsi` or `virtio` disk. Defaults to `false`.
	IOThread bool `mapstructure:"io_thread"`
	// Relay TRIM commands to the underlying storage. Defaults
	// to false. See the
	// [Proxmox documentation](https://pve.proxmox.com/pve-docs/pve-admin-guide.html#qm_hard_disk_discard)
	// for for further information.
	Discard bool `mapstructure:"discard"`
	// Drive will be presented to the guest as solid-state drive
	// rather than a rotational disk.
	SSD bool `mapstructure:"ssd"`
}

// - `efi_config` - (object) - Set the efidisk storage options. This needs to be set if you use ovmf uefi boot
//   (supersedes the `efidisk` option).
//
//   Usage example (JSON):
//
//   ```json
//   {
//     "efi_storage_pool": "local",
//     "pre_enrolled_keys": true,
//     "efi_type": "4m"
//   }
//   ```
type efiConfig struct {
	// Name of the Proxmox storage pool to store the EFI disk on.
	EFIStoragePool string `mapstructure:"efi_storage_pool"`
	// Whether Microsoft Standard Secure Boot keys should be pre-loaded on
	// the EFI disk. Defaults to `false`.
	PreEnrolledKeys bool `mapstructure:"pre_enrolled_keys"`
	// Specifies the version of the OVMF firmware to be used. Can be `2m` or `4m`.
	// Defaults to `4m`.
	EFIType string `mapstructure:"efi_type"`
}

// - `rng0` (object): Configure Random Number Generator via VirtIO.
// A virtual hardware-RNG can be used to provide entropy from the host system to a guest VM helping avoid entropy starvation which might cause the guest system slow down.
// The device is sourced from a host device and guest, his use can be limited: `max_bytes` bytes of data will become available on a `period` ms timer.
// [PVE documentation](https://pve.proxmox.com/pve-docs/pve-admin-guide.html) recommends to always use a limiter to avoid guests using too many host resources.
//
// HCL2 example:
//
// ```hcl
//
//	rng0 {
//	  source    = "/dev/urandom"
//	  max_bytes = 1024
//	  period    = 1000
//	}
//
// ```
//
// JSON example:
//
// ```json
//
//	{
//	    "rng0": {
//	        "source": "/dev/urandom",
//	        "max_bytes": 1024,
//	        "period": 1000
//	    }
//	}
//
// ```
type rng0Config struct {
	// Device on the host to gather entropy from.
	// `/dev/urandom` should be preferred over `/dev/random` as Proxmox PVE documentation suggests.
	// `/dev/hwrng` can be used to pass through a hardware RNG.
	// Can be one of `/dev/urandom`, `/dev/random`, `/dev/hwrng`.
	Source string `mapstructure:"source" required:"true"`
	// Maximum bytes of entropy allowed to get injected into the guest every `period` milliseconds.
	// Use a lower value when using `/dev/random` since can lead to entropy starvation on the host system.
	// `0` disables limiting and according to PVE documentation is potentially dangerous for the host.
	// Recommended value: `1024`.
	MaxBytes int `mapstructure:"max_bytes" required:"true"`
	// Period in milliseconds on which the the entropy-injection quota is reset.
	// Can be a positive value.
	// Recommended value: `1000`.
	Period int `mapstructure:"period" required:"false"`
}

type vgaConfig struct {
	Type   string `mapstructure:"type"`
	Memory int    `mapstructure:"memory"`
}

// Allows passing through a host PCI device into the VM. For example, a graphics card
// or a network adapter. Devices that are mapped into a guest VM are no longer available
// on the host. A minimal configuration only requires either the `host` or the `mapping`
// key to be specifed.
//
// Note: VMs with passed-through devices cannot be migrated.
//
// HCL2 example:
//
// ```hcl
//
//	pci_devices {
//	  host          = "0000:0d:00.1"
//	  pcie          = false
//	  device_id     = "1003"
//	  legacy_igd    = false
//	  mdev          = "some-model"
//	  hide_rombar   = false
//	  romfile       = "vbios.bin"
//	  sub_device_id = ""
//	  sub_vendor_id = ""
//	  vendor_id     = "15B3"
//	  x_vga         = false
//	}
//
// ```
//
// JSON example:
//
// ```json
//
//	{
//	  "pci_devices": {
//	    "host"          : "0000:0d:00.1",
//	    "pcie"          : false,
//	    "device_id"     : "1003",
//	    "legacy_igd"    : false,
//	    "mdev"          : "some-model",
//	    "hide_rombar"   : false,
//	    "romfile"       : "vbios.bin",
//	    "sub_device_id" : "",
//	    "sub_vendor_id" : "",
//	    "vendor_id"     : "15B3",
//	    "x_vga"         : false
//	  }
//	}
//
// ```
type pciDeviceConfig struct {
	// The PCI ID of a host’s PCI device or a PCI virtual function. You can us the `lspci` command to list existing PCI devices. Either this or the `mapping` key must be set.
	Host string `mapstructure:"host"`
	// Override PCI device ID visible to guest.
	DeviceID string `mapstructure:"device_id"`
	// Pass this device in legacy IGD mode, making it the primary and exclusive graphics device in the VM. Requires `pc-i440fx` machine type and VGA set to `none`. Defaults to `false`.
	LegacyIGD bool `mapstructure:"legacy_igd"`
	// The ID of a cluster wide mapping. Either this or the `host` key must be set.
	Mapping string `mapstructure:"mapping"`
	// Present the device as a PCIe device (needs `q35` machine model). Defaults to `false`.
	PCIe bool `mapstructure:"pcie"`
	// The type of mediated device to use. An instance of this type will be created on startup of the VM and will be cleaned up when the VM stops.
	MDEV string `mapstructure:"mdev"`
	// Specify whether or not the device’s ROM BAR will be visible in the guest’s memory map. Defaults to `false`.
	HideROMBAR bool `mapstructure:"hide_rombar"`
	// Custom PCI device rom filename (must be located in `/usr/share/kvm/`).
	ROMFile string `mapstructure:"romfile"`
	//Override PCI subsystem device ID visible to guest.
	SubDeviceID string `mapstructure:"sub_device_id"`
	// Override PCI subsystem vendor ID visible to guest.
	SubVendorID string `mapstructure:"sub_vendor_id"`
	// Override PCI vendor ID visible to guest.
	VendorID string `mapstructure:"vendor_id"`
	// Enable vfio-vga device support. Defaults to `false`.
	XVGA bool `mapstructure:"x_vga"`
}

func (c *Config) Prepare(upper interface{}, raws ...interface{}) ([]string, []string, error) {
	// Do not add a cloud-init cdrom by default
	c.CloudInit = false
	var md mapstructure.Metadata
	err := config.Decode(upper, &config.DecodeOpts{
		Metadata:           &md,
		Interpolate:        true,
		InterpolateContext: &c.Ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{
				"boot_command",
			},
		},
	}, raws...)
	if err != nil {
		return nil, nil, err
	}

	var errs *packersdk.MultiError
	var warnings []string

	if c.Ctx.BuildType == "proxmox" {
		warnings = append(warnings, "proxmox is deprecated, please use proxmox-iso instead")
	}

	// Default qemu_agent to true
	if c.Agent != config.TriFalse {
		c.Agent = config.TriTrue
	}

	packersdk.LogSecretFilter.Set(c.Password)

	// Defaults
	if c.ProxmoxURLRaw == "" {
		c.ProxmoxURLRaw = os.Getenv("PROXMOX_URL")
	}
	if c.Username == "" {
		c.Username = os.Getenv("PROXMOX_USERNAME")
	}
	if c.Password == "" {
		c.Password = os.Getenv("PROXMOX_PASSWORD")
	}
	if c.Token == "" {
		c.Token = os.Getenv("PROXMOX_TOKEN")
	}
	if c.TaskTimeout == 0 {
		c.TaskTimeout = 60 * time.Second
	}
	if c.BootKeyInterval == 0 && os.Getenv(bootcommand.PackerKeyEnv) != "" {
		var err error
		c.BootKeyInterval, err = time.ParseDuration(os.Getenv(bootcommand.PackerKeyEnv))
		if err != nil {
			errs = packersdk.MultiErrorAppend(errs, err)
		}
	}
	if c.BootKeyInterval == 0 {
		c.BootKeyInterval = 5 * time.Millisecond
	}

	// Technically Proxmox VMIDs are unsigned 32bit integers, but are limited to
	// the range 100-999999999. Source:
	// https://pve-devel.pve.proxmox.narkive.com/Pa6mH1OP/avoiding-vmid-reuse#post8
	if c.VMID != 0 && (c.VMID < 100 || c.VMID > 999999999) {
		errs = packersdk.MultiErrorAppend(errs, errors.New("vm_id must be in range 100-999999999"))
	}
	if c.VMName == "" {
		// Default to packer-[time-ordered-uuid]
		c.VMName = fmt.Sprintf("packer-%s", uuid.TimeOrderedUUID())
	}
	if c.Memory < 16 {
		log.Printf("Memory %d is too small, using default: 512", c.Memory)
		c.Memory = 512
	}
	if c.Memory < c.BalloonMinimum {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("ballooning_minimum (%d) must be lower than memory (%d)", c.BalloonMinimum, c.Memory))
	}
	if c.Cores < 1 {
		log.Printf("Number of cores %d is too small, using default: 1", c.Cores)
		c.Cores = 1
	}
	if c.Sockets < 1 {
		log.Printf("Number of sockets %d is too small, using default: 1", c.Sockets)
		c.Sockets = 1
	}
	if c.CPUType == "" {
		log.Printf("CPU type not set, using default 'kvm64'")
		c.CPUType = "kvm64"
	}
	if c.OS == "" {
		log.Printf("OS not set, using default 'other'")
		c.OS = "other"
	}
	for idx, disk := range c.Disks {
		if disk.Type == "" {
			log.Printf("Disk %d type not set, using default 'scsi'", idx)
			c.Disks[idx].Type = "scsi"
		}
		if disk.Size == "" {
			log.Printf("Disk %d size not set, using default '20G'", idx)
			c.Disks[idx].Size = "20G"
		}
		if disk.CacheMode == "" {
			log.Printf("Disk %d cache mode not set, using default 'none'", idx)
			c.Disks[idx].CacheMode = "none"
		}
		if disk.IOThread {
			// io thread is only supported by virtio-scsi-single controller
			if c.SCSIController != "virtio-scsi-single" {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("io thread option requires virtio-scsi-single controller"))
			} else {
				// ... and only for virtio and scsi disks
				if !(disk.Type == "scsi" || disk.Type == "virtio") {
					errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("io thread option requires scsi or a virtio disk"))
				}
			}
		}
		if disk.StoragePool == "" {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("disks[%d].storage_pool must be specified", idx))
		}
		if disk.StoragePoolType != "" {
			warnings = append(warnings, "storage_pool_type is deprecated and should be omitted, it will be removed in a later version of the proxmox plugin")
		}
	}
	if len(c.Serials) > 4 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("too many serials: %d serials defined, but proxmox accepts 4 elements maximum", len(c.Serials)))
	}
	res := regexp.MustCompile(`^(/dev/.+|socket)$`)
	for _, serial := range c.Serials {
		if !res.MatchString(serial) {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("serials must respond to pattern \"/dev/.+\" or be \"socket\". It was \"%s\"", serial))
		}
	}
	if c.SCSIController == "" {
		log.Printf("SCSI controller not set, using default 'lsi'")
		c.SCSIController = "lsi"
	}

	errs = packersdk.MultiErrorAppend(errs, c.Comm.Prepare(&c.Ctx)...)
	errs = packersdk.MultiErrorAppend(errs, c.BootConfig.Prepare(&c.Ctx)...)
	errs = packersdk.MultiErrorAppend(errs, c.HTTPConfig.Prepare(&c.Ctx)...)

	// Required configurations that will display errors if not set
	if c.Username == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("username must be specified"))
	}
	if c.Password == "" && c.Token == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("password or token must be specified"))
	}
	if c.ProxmoxURLRaw == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("proxmox_url must be specified"))
	}
	if c.proxmoxURL, err = url.Parse(c.ProxmoxURLRaw); err != nil {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse proxmox_url: %s", err))
	}
	if c.Node == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("node must be specified"))
	}

	// Verify VM Name and Template Name are a valid DNS Names
	re := regexp.MustCompile(`^(?:(?:(?:[a-zA-Z0-9](?:[a-zA-Z0-9\-]*[a-zA-Z0-9])?)\.)*(?:[A-Za-z0-9](?:[A-Za-z0-9\-]*[A-Za-z0-9])?))$`)
	if !re.MatchString(c.VMName) {
		errs = packersdk.MultiErrorAppend(errs, errors.New("vm_name must be a valid DNS name"))
	}
	if c.TemplateName != "" && !re.MatchString(c.TemplateName) {
		errs = packersdk.MultiErrorAppend(errs, errors.New("template_name must be a valid DNS name"))
	}
	for idx, nic := range c.NICs {
		if nic.Bridge == "" {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("network_adapters[%d].bridge must be specified", idx))
		}
		if nic.Model == "" {
			log.Printf("NIC %d model not set, using default 'e1000'", idx)
			c.NICs[idx].Model = "e1000"
		}
		if nic.Model != "virtio" && nic.PacketQueues > 0 {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("network_adapters[%d].packet_queues can only be set for 'virtio' driver", idx))
		}
		if (nic.MTU < 0) || (nic.MTU > 65520) {
			errs = packersdk.MultiErrorAppend(errs, errors.New("network_adapters[%d].mtu only positive values up to 65520 are supported"))
		}
	}
	for idx := range c.AdditionalISOFiles {
		// Check AdditionalISO config
		// Either a pre-uploaded ISO should be referenced in iso_file, OR a URL
		// (possibly to a local file) to an ISO file that will be downloaded and
		// then uploaded to Proxmox.
		if c.AdditionalISOFiles[idx].ISOFile != "" {
			c.AdditionalISOFiles[idx].ShouldUploadISO = false
		} else {
			c.AdditionalISOFiles[idx].DownloadPathKey = "downloaded_additional_iso_path_" + strconv.Itoa(idx)
			if len(c.AdditionalISOFiles[idx].CDFiles) > 0 || len(c.AdditionalISOFiles[idx].CDContent) > 0 {
				cdErrors := c.AdditionalISOFiles[idx].CDConfig.Prepare(&c.Ctx)
				errs = packersdk.MultiErrorAppend(errs, cdErrors...)
			} else {
				isoWarnings, isoErrors := c.AdditionalISOFiles[idx].ISOConfig.Prepare(&c.Ctx)
				errs = packersdk.MultiErrorAppend(errs, isoErrors...)
				warnings = append(warnings, isoWarnings...)
			}
			c.AdditionalISOFiles[idx].ShouldUploadISO = true
		}
		if c.AdditionalISOFiles[idx].Device == "" {
			log.Printf("AdditionalISOFile %d Device not set, using default 'ide3'", idx)
			c.AdditionalISOFiles[idx].Device = "ide3"
		}
		if strings.HasPrefix(c.AdditionalISOFiles[idx].Device, "ide") {
			busnumber, err := strconv.Atoi(c.AdditionalISOFiles[idx].Device[3:])
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("%s is not a valid bus index", c.AdditionalISOFiles[idx].Device[3:]))
			}
			if busnumber == 2 {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("IDE bus 2 is used by boot ISO"))
			}
			if busnumber > 3 {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("IDE bus index can't be higher than 3"))
			}
		}
		if strings.HasPrefix(c.AdditionalISOFiles[idx].Device, "sata") {
			busnumber, err := strconv.Atoi(c.AdditionalISOFiles[idx].Device[4:])
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("%s is not a valid bus index", c.AdditionalISOFiles[idx].Device[4:]))
			}
			if busnumber > 5 {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("SATA bus index can't be higher than 5"))
			}
		}
		if strings.HasPrefix(c.AdditionalISOFiles[idx].Device, "scsi") {
			busnumber, err := strconv.Atoi(c.AdditionalISOFiles[idx].Device[4:])
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("%s is not a valid bus index", c.AdditionalISOFiles[idx].Device[4:]))
			}
			if busnumber > 30 {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("SCSI bus index can't be higher than 30"))
			}
		}
		if len(c.AdditionalISOFiles[idx].CDFiles) > 0 || len(c.AdditionalISOFiles[idx].CDContent) > 0 {
			if c.AdditionalISOFiles[idx].ISOStoragePool == "" {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("iso_storage_pool not set for storage of generated ISO from cd_files or cd_content"))
			}
		}
		// Check only one option is present
		options := 0
		if c.AdditionalISOFiles[idx].ISOFile != "" {
			options++
		}
		if len(c.AdditionalISOFiles[idx].ISOConfig.ISOUrls) > 0 || c.AdditionalISOFiles[idx].ISOConfig.RawSingleISOUrl != "" {
			options++
		}
		if len(c.AdditionalISOFiles[idx].CDFiles) > 0 || len(c.AdditionalISOFiles[idx].CDContent) > 0 {
			options++
		}
		if options != 1 {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("one of iso_file, iso_url, or a combination of cd_files and cd_content must be specified for AdditionalISO file %s", c.AdditionalISOFiles[idx].Device))
		}
	}
	if c.EFIDisk != "" {
		if c.EFIConfig != (efiConfig{}) {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("both efi_config and efidisk cannot be set at the same time, consider defining only efi_config as efidisk is deprecated"))
		} else {
			warnings = append(warnings, "efidisk is deprecated, please use efi_config instead")
			c.EFIConfig.EFIStoragePool = c.EFIDisk
		}
	}
	if c.EFIConfig.EFIStoragePool != "" {
		if c.EFIConfig.EFIType == "" {
			log.Printf("EFI disk defined, but no efi_type given, using 4m")
			c.EFIConfig.EFIType = "4m"
		}
	} else {
		if c.EFIConfig.EFIType != "" || c.EFIConfig.PreEnrolledKeys {
			errs = packersdk.MultiErrorAppend(errs, errors.New("efi_storage_pool not set for efi_config"))
		}
	}
	if c.Rng0 != (rng0Config{}) {
		if !(c.Rng0.Source == "/dev/urandom" || c.Rng0.Source == "/dev/random" || c.Rng0.Source == "/dev/hwrng") {
			errs = packersdk.MultiErrorAppend(errs, errors.New("source must be one of \"/dev/urandom\", \"/dev/random\", \"/dev/hwrng\""))
		}
		if c.Rng0.MaxBytes < 0 {
			errs = packersdk.MultiErrorAppend(errs, errors.New("max_bytes must be >= 0"))
		} else {
			if c.Rng0.MaxBytes == 0 {
				warnings = append(warnings, "max_bytes is 0: potentially dangerous: this disables limiting the entropy allowed to get injected into the guest")
			}
		}
		if c.Rng0.Period < 0 {
			errs = packersdk.MultiErrorAppend(errs, errors.New("period must be >= 0"))
		}
	}

	// See https://pve.proxmox.com/pve-docs/api-viewer/index.html#/nodes/{node}/hardware/pci/{pciid}
	validPCIIDre := regexp.MustCompile(`^(?:[0-9a-fA-F]{4}:)?[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-9a-fA-F]$`)
	for _, device := range c.PCIDevices {
		if device.Host == "" && device.Mapping == "" {
			errs = packersdk.MultiErrorAppend(errs, errors.New("either the host or the mapping key must be specified"))
		}
		if device.Host != "" && device.Mapping != "" {
			errs = packersdk.MultiErrorAppend(errs, errors.New("the host and the mapping key cannot both be set"))
		}
		if device.Host != "" && !validPCIIDre.MatchString(device.Host) {
			errs = packersdk.MultiErrorAppend(errs, errors.New("host contains invalid PCI ID"))
		}
		if device.LegacyIGD {
			if c.Machine != "pc" && !strings.HasPrefix(c.Machine, "pc-i440fx") {
				errs = packersdk.MultiErrorAppend(errs, errors.New("legacy_igd requires pc-i440fx machine type"))
			}
			if c.VGA.Type != "none" {
				errs = packersdk.MultiErrorAppend(errs, errors.New("legacy_igd requires vga.type set to none"))
			}
		}
		if device.PCIe {
			if c.Machine != "q35" && !strings.HasPrefix(c.Machine, "pc-q35") {
				errs = packersdk.MultiErrorAppend(errs, errors.New("pcie requires q35 machine type"))
			}
		}
	}

	if errs != nil && len(errs.Errors) > 0 {
		return nil, warnings, errs
	}
	return nil, warnings, nil
}
