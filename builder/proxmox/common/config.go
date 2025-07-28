// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,NICConfig,diskConfig,rng0Config,pciDeviceConfig,vgaConfig,ISOsConfig,efiConfig,tpmConfig

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

// There are many configuration options available for the builder. They are
// segmented below into two categories: required and optional parameters. Within
// each category, the available configuration keys are alphabetized.
//
// You may also want to take look at the general configuration references for
// [VirtIO RNG device](#virtio-rng-device)
// and [PCI Devices](#pci-devices)
// configuration references, which can be found further down the page.
//
// In addition to the options listed here, a
// [communicator](/packer/docs/templates/legacy_json_templates/communicator) can be configured for this
// builder.
//
// If no communicator is defined, an SSH key is generated for use, and is used
// in the image's Cloud-Init settings for provisioning.
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	commonsteps.HTTPConfig `mapstructure:",squash"`
	bootcommand.BootConfig `mapstructure:",squash"`
	BootKeyInterval        time.Duration       `mapstructure:"boot_key_interval"`
	Comm                   communicator.Config `mapstructure:",squash"`

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
	// Which node in the Proxmox cluster to start the virtual
	// machine on during creation.
	Node string `mapstructure:"node"`
	// Name of resource pool to create virtual machine in.
	Pool string `mapstructure:"pool"`
	// `task_timeout` (duration string | ex: "10m") - The timeout for
	//  Promox API operations, e.g. clones. Defaults to 1 minute.
	TaskTimeout time.Duration `mapstructure:"task_timeout"`

	// Name of the virtual machine during creation. If not
	// given, a random uuid will be used.
	VMName string `mapstructure:"vm_name"`
	// `vm_id` (int) - The ID used to reference the virtual machine. This will
	// also be the ID of the final template. Proxmox VMIDs are unique cluster-wide
	// and are limited to the range 100-999999999.
	// If not given, the next free ID on the cluster will be used.
	VMID int `mapstructure:"vm_id"`

	// The tags to set. This is a semicolon separated list. For example,
	// `debian-12;template`.
	Tags string `mapstructure:"tags"`

	// Override default boot order. Format example `order=virtio0;ide2;net0`.
	// Prior to Proxmox 6.2-15 the format was `cdn` (c:CDROM -> d:Disk -> n:Network)
	Boot string `mapstructure:"boot"`
	// How much memory (in megabytes) to give the virtual
	// machine. If `ballooning_minimum` is also set, `memory` defines the maximum amount
	// of memory the VM will be able to use.
	// Defaults to `512`.
	Memory uint32 `mapstructure:"memory"`
	// Setting this option enables KVM memory ballooning and
	// defines the minimum amount of memory (in megabytes) the VM will have.
	// Defaults to `0` (memory ballooning disabled).
	BalloonMinimum uint32 `mapstructure:"ballooning_minimum"`
	// How many CPU cores to give the virtual machine. Defaults
	// to `1`.
	Cores uint8 `mapstructure:"cores"`
	// The CPU type to emulate. See the Proxmox API
	// documentation for the complete list of accepted values. For best
	// performance, set this to `host`. Defaults to `kvm64`.
	CPUType string `mapstructure:"cpu_type"`
	// How many CPU sockets to give the virtual machine.
	// Defaults to `1`
	Sockets uint8 `mapstructure:"sockets"`
	// If true, support for non-uniform memory access (NUMA)
	// is enabled. Defaults to `false`.
	Numa bool `mapstructure:"numa"`
	// The operating system. Can be `wxp`, `w2k`, `w2k3`, `w2k8`,
	// `wvista`, `win7`, `win8`, `win10`, `l24` (Linux 2.4), `l26` (Linux 2.6+),
	// `solaris` or `other`. Defaults to `other`.
	OS string `mapstructure:"os"`
	// Set the machine bios. This can be set to ovmf or seabios. The default value is seabios.
	BIOS string `mapstructure:"bios"`
	// Set the efidisk storage options. See [EFI Config](#efi-config).
	EFIConfig efiConfig `mapstructure:"efi_config"`
	// This option is deprecated, please use `efi_config` instead.
	EFIDisk string `mapstructure:"efidisk"`
	// Set the machine type. Supported values are 'pc' or 'q35'.
	Machine string `mapstructure:"machine"`
	// Configure Random Number Generator via VirtIO. See [VirtIO RNG device](#virtio-rng-device)
	Rng0 rng0Config `mapstructure:"rng0"`
	// Set the tpmstate storage options. See [TPM Config](#tpm-config).
	TPMConfig tpmConfig `mapstructure:"tpm_config"`
	// The graphics adapter to use. See [VGA Config](#vga-config).
	VGA vgaConfig `mapstructure:"vga"`
	// The network adapter to use. See [Network Adapters](#network-adapters)
	NICs []NICConfig `mapstructure:"network_adapters"`
	// Disks attached to the virtual machine. See [Disks](#disks)
	Disks []diskConfig `mapstructure:"disks"`
	// Allows passing through a host PCI device into the VM. See [PCI Devices](#pci-devices)
	PCIDevices []pciDeviceConfig `mapstructure:"pci_devices"`
	// A list (max 4 elements) of serial ports attached to
	// the virtual machine. It may pass through a host serial device `/dev/ttyS0`
	// or create unix socket on the host `socket`. Each element can be `socket`
	// or responding to pattern `/dev/.+`. Example:
	//
	//   ```json
	//   [
	//     "socket",
	//     "/dev/ttyS1"
	//   ]
	//   ```
	Serials []string `mapstructure:"serials"`
	// Enables QEMU Agent option for this VM. When enabled,
	// then `qemu-guest-agent` must be installed on the guest. When disabled, then
	// `ssh_host` should be used. Defaults to `true`.
	Agent config.Trilean `mapstructure:"qemu_agent"`
	// The SCSI controller model to emulate. Can be `lsi`,
	// `lsi53c810`, `virtio-scsi-pci`, `virtio-scsi-single`, `megasas`, or `pvscsi`.
	// Defaults to `lsi`.
	SCSIController string `mapstructure:"scsi_controller"`
	// Specifies whether a VM will be started during system
	// bootup. Defaults to `false`.
	Onboot bool `mapstructure:"onboot"`
	// Disables KVM hardware virtualization. Defaults to `false`.
	DisableKVM bool `mapstructure:"disable_kvm"`

	// Name of the template. Defaults to the generated
	// name used during creation.
	TemplateName string `mapstructure:"template_name"`
	// Description of the template, visible in
	// the Proxmox interface.
	TemplateDescription string `mapstructure:"template_description"`

	// If true, add an empty Cloud-Init CDROM drive after the virtual
	// machine has been converted to a template. Defaults to `false`.
	CloudInit bool `mapstructure:"cloud_init"`
	// Name of the Proxmox storage pool
	// to store the Cloud-Init CDROM on. If not given, the storage pool of the boot device will be used.
	CloudInitStoragePool string `mapstructure:"cloud_init_storage_pool"`
	// The type of Cloud-Init disk. Can be `scsi`, `sata`, or `ide`
	// Defaults to `ide`.
	CloudInitDiskType string `mapstructure:"cloud_init_disk_type"`

	// ISO files attached to the virtual machine.
	// See [ISOs](#isos).
	ISOs []ISOsConfig `mapstructure:"additional_iso_files"`
	// Name of the network interface that Packer gets
	// the VMs IP from. Defaults to the first non loopback interface.
	VMInterface string `mapstructure:"vm_interface"`

	// Arbitrary arguments passed to KVM.
	// For example `-no-reboot -smbios type=0,vendor=FOO`.
	// 	Note: this option is for experts only.
	AdditionalArgs string `mapstructure:"qemu_additional_args"`

	// Used by clone builder StepMapSourceDisks to store existing disk assignments
	CloneSourceDisks []string `mapstructure-to-hcl2:",skip"`

	Ctx interpolate.Context `mapstructure-to-hcl2:",skip"`
}

// ISO files attached to the virtual machine.
//
// JSON Example:
//
// ```json
//
//	"additional_iso_files": [
//		{
//			  "type": "scsi",
//			  "iso_file": "local:iso/virtio-win-0.1.185.iso",
//			  "unmount": true,
//			  "iso_checksum": "af2b3cc9fa7905dea5e58d31508d75bba717c2b0d5553962658a47aebc9cc386"
//		}
//	 ]
//
// ```
// HCL2 example:
//
// ```hcl
//
//	additional_iso_files {
//	  type = "scsi"
//	  iso_file = "local:iso/virtio-win-0.1.185.iso"
//	  unmount = true
//	  iso_checksum = "af2b3cc9fa7905dea5e58d31508d75bba717c2b0d5553962658a47aebc9cc386"
//	}
//
// ```
type ISOsConfig struct {
	commonsteps.ISOConfig `mapstructure:",squash"`
	// DEPRECATED. Assign bus type with `type`. Optionally assign a bus index with `index`.
	// Bus type and bus index that the ISO will be mounted on. Can be `ideX`,
	// `sataX` or `scsiX`.
	// For `ide` the bus index ranges from 0 to 3, for `sata` from 0 to 5 and for
	// `scsi` from 0 to 30.
	// Defaulted to `ide3` in versions up to v1.8, now defaults to dynamic ide assignment (next available ide bus index after hard disks are allocated)
	Device string `mapstructure:"device"`
	// Bus type that the ISO will be mounted on. Can be `ide`, `sata` or `scsi`. Defaults to `ide`.
	Type string `mapstructure:"type"`
	// Optional: Used in combination with `type` to statically assign an ISO to a bus index.
	Index string `mapstructure:"index"`
	// Path to the ISO file to boot from, expressed as a
	// proxmox datastore path, for example
	// `local:iso/Fedora-Server-dvd-x86_64-29-1.2.iso`.
	// Either `iso_file` OR `iso_url` must be specifed.
	ISOFile string `mapstructure:"iso_file"`
	// Proxmox storage pool onto which to upload
	// the ISO file.
	ISOStoragePool string `mapstructure:"iso_storage_pool"`
	// Download the ISO directly from the PVE node rather than through Packer.
	//
	// Defaults to `false`
	ISODownloadPVE bool `mapstructure:"iso_download_pve"`
	// If true, remove the mounted ISO from the template after finishing. Defaults to `false`.
	Unmount bool `mapstructure:"unmount"`
	// Keep CDRom device attached to template if unmounting ISO. Defaults to `false`.
	// Has no effect if unmount is `false`
	KeepCDRomDevice      bool   `mapstructure:"keep_cdrom_device"`
	ShouldUploadISO      bool   `mapstructure-to-hcl2:",skip"`
	DownloadPathKey      string `mapstructure-to-hcl2:",skip"`
	AssignedDeviceIndex  string `mapstructure-to-hcl2:",skip"`
	commonsteps.CDConfig `mapstructure:",squash"`
}

// Network adapters attached to the virtual machine.
//
// Example:
//
// ```json
// [
//
//	{
//	  "model": "virtio",
//	  "bridge": "vmbr0",
//	  "vlan_tag": "10",
//	  "firewall": true
//	}
//
// ]
// ```
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

// Disks attached to the virtual machine.
//
// Example:
//
// ```json
// [
//
//	{
//	  "type": "scsi",
//	  "disk_size": "5G",
//	  "storage_pool": "local-lvm",
//	  "storage_pool_type": "lvm"
//	}
//
// ]
// ```
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
	// Configure Asynchronous I/O. Can be `native`, `threads`, or `io_uring`.
	// Defaults to io_uring.
	AsyncIO string `mapstructure:"asyncio"`
	// Exclude disk from Proxmox backup jobs
	// Defaults to false.
	ExcludeFromBackup bool `mapstructure:"exclude_from_backup"`
	// Relay TRIM commands to the underlying storage. Defaults
	// to false. See the
	// [Proxmox documentation](https://pve.proxmox.com/pve-docs/pve-admin-guide.html#qm_hard_disk_discard)
	// for for further information.
	Discard bool `mapstructure:"discard"`
	// Drive will be presented to the guest as solid-state drive
	// rather than a rotational disk.
	//
	// This cannot work with virtio disks.
	SSD bool `mapstructure:"ssd"`
	// Exclude disk from replication jobs.
	// Defaults to false.
	SkipReplication bool `mapstructure:"skip_replication"`
}

// Set the efidisk storage options.
// This needs to be set if you use ovmf uefi boot (supersedes the `efidisk` option).
//
// Usage example (JSON):
//
// ```json
//
//	{
//	  "efi_storage_pool": "local",
//	  "pre_enrolled_keys": true,
//	  "efi_format": "raw",
//	  "efi_type": "4m"
//	}
//
// ```
type efiConfig struct {
	// Name of the Proxmox storage pool to store the EFI disk on.
	EFIStoragePool string `mapstructure:"efi_storage_pool"`
	// The format of the file backing the disk. Can be
	// `raw`, `cow`, `qcow`, `qed`, `qcow2`, `vmdk` or `cloop`. Defaults to
	// `raw`.
	EFIFormat string `mapstructure:"efi_format"`
	// Whether Microsoft Standard Secure Boot keys should be pre-loaded on
	// the EFI disk. Defaults to `false`.
	PreEnrolledKeys bool `mapstructure:"pre_enrolled_keys"`
	// Specifies the version of the OVMF firmware to be used. Can be `2m` or `4m`.
	// Defaults to `4m`.
	EFIType string `mapstructure:"efi_type"`
}

// Set the tpmstate storage options.
//
// HCL2 example:
//
// ```hcl
//
//	tpm_config {
//	  tpm_storage_pool = "local"
//	  tpm_version      = "v1.2"
//	}
//
// ```
// Usage example (JSON):
//
// ```json
//
//	"tpm_config": {
//	  "tpm_storage_pool": "local",
//	  "tpm_version": "v1.2"
//	}
//
// ```
type tpmConfig struct {
	// Name of the Proxmox storage pool to store the EFI disk on.
	TPMStoragePool string `mapstructure:"tpm_storage_pool"`
	// Version of TPM spec. Can be `v1.2` or `v2.0` Defaults to `v2.0`.
	Version string `mapstructure:"tpm_version"`
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

// - `vga` (object) - The graphics adapter to use. Example:
//
//	```json
//	{
//	  "type": "vmware",
//	  "memory": 32
//	}
//	```
type vgaConfig struct {
	// Can be `cirrus`, `none`, `qxl`,`qxl2`, `qxl3`,
	// `qxl4`, `serial0`, `serial1`, `serial2`, `serial3`, `std`, `virtio`, `vmware`.
	// Defaults to `std`.
	Type string `mapstructure:"type"`
	// How much memory to assign.
	Memory int `mapstructure:"memory"`
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
	// validate iso devices
	for idx := range c.ISOs {
		// Check ISO config
		// Either a pre-uploaded ISO should be referenced in iso_file, OR a URL
		// (possibly to a local file) to an ISO file that will be downloaded and
		// then uploaded to Proxmox.
		if c.ISOs[idx].ISOFile != "" {
			// ISOFile should match <storage>:iso/<ISO filename> format
			res := regexp.MustCompile(`^.+:iso\/.+$`)
			if !res.MatchString(c.ISOs[idx].ISOFile) {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("iso_path should match pattern \"<storage>:iso/<ISO filename>\". Provided value was \"%s\"", c.ISOs[idx].ISOFile))
			}
			c.ISOs[idx].ShouldUploadISO = false
		} else {
			c.ISOs[idx].DownloadPathKey = "downloaded_additional_iso_path_" + strconv.Itoa(idx)
			if len(c.ISOs[idx].CDFiles) > 0 || len(c.ISOs[idx].CDContent) > 0 {
				cdErrors := c.ISOs[idx].CDConfig.Prepare(&c.Ctx)
				errs = packersdk.MultiErrorAppend(errs, cdErrors...)
			} else {
				isoWarnings, isoErrors := c.ISOs[idx].ISOConfig.Prepare(&c.Ctx)
				errs = packersdk.MultiErrorAppend(errs, isoErrors...)
				warnings = append(warnings, isoWarnings...)
			}
			c.ISOs[idx].ShouldUploadISO = true
		}
		// validate device field
		if c.ISOs[idx].Device != "" {
			warnings = append(warnings, "additional_iso_files field 'device' is deprecated and will be removed in a future release, assign bus type with 'type'. Optionally assign a bus index with 'index'")
			if strings.HasPrefix(c.ISOs[idx].Device, "ide") {
				busnumber, err := strconv.Atoi(c.ISOs[idx].Device[3:])
				if err != nil {
					errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("%s is not a valid bus index", c.ISOs[idx].Device[3:]))
				}
				if busnumber > 3 {
					errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("IDE bus index can't be higher than 3"))
				} else {
					// convert device field to type and index fields
					log.Printf("converting deprecated field 'device' value %s to 'type' %s and 'index' %d", c.ISOs[idx].Device, "ide", busnumber)
					c.ISOs[idx].Type = "ide"
					c.ISOs[idx].Index = strconv.Itoa(busnumber)
				}
			}
			if strings.HasPrefix(c.ISOs[idx].Device, "sata") {
				busnumber, err := strconv.Atoi(c.ISOs[idx].Device[4:])
				if err != nil {
					errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("%s is not a valid bus index", c.ISOs[idx].Device[4:]))
				}
				if busnumber > 5 {
					errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("SATA bus index can't be higher than 5"))
				} else {
					// convert device field to type and index fields
					log.Printf("converting deprecated field 'device' value %s to 'type' %s and 'index' %d", c.ISOs[idx].Device, "sata", busnumber)
					c.ISOs[idx].Type = "sata"
					c.ISOs[idx].Index = strconv.Itoa(busnumber)
				}
			}
			if strings.HasPrefix(c.ISOs[idx].Device, "scsi") {
				busnumber, err := strconv.Atoi(c.ISOs[idx].Device[4:])
				if err != nil {
					errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("%s is not a valid bus index", c.ISOs[idx].Device[4:]))
				}
				if busnumber > 30 {
					errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("SCSI bus index can't be higher than 30"))
				} else {
					// convert device field to type and index fields
					log.Printf("converting deprecated field 'device' value %s to 'type' %s and 'index' %d", c.ISOs[idx].Device, "scsi", busnumber)
					c.ISOs[idx].Type = "scsi"
					c.ISOs[idx].Index = strconv.Itoa(busnumber)
				}
			}
		}
		// validate device type, assign if unset
		switch c.ISOs[idx].Type {
		case "ide", "sata", "scsi":
		case "":
			log.Printf("additional_iso %d device type not set, using default 'ide'", idx)
			c.ISOs[idx].Type = "ide"
		default:
			errs = packersdk.MultiErrorAppend(errs, errors.New("ISOs must be of type ide, sata or scsi. VirtIO not supported by Proxmox for ISO devices"))
		}
		if len(c.ISOs[idx].CDFiles) > 0 || len(c.ISOs[idx].CDContent) > 0 {
			if c.ISOs[idx].ISOStoragePool == "" {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("storage_pool not set for storage of generated ISO from cd_files or cd_content"))
			}
		}
		if len(c.ISOs[idx].ISOUrls) != 0 && c.ISOs[idx].ISOStoragePool == "" {
			errs = packersdk.MultiErrorAppend(errs, errors.New("when specifying iso_url in an additional_isos block, iso_storage_pool must also be specified"))
		}
		// Check only one option is present
		options := 0
		if c.ISOs[idx].ISOFile != "" {
			options++
		}
		if len(c.ISOs[idx].ISOConfig.ISOUrls) > 0 || c.ISOs[idx].ISOConfig.RawSingleISOUrl != "" {
			options++
		}
		if len(c.ISOs[idx].CDFiles) > 0 || len(c.ISOs[idx].CDContent) > 0 {
			options++
		}
		if options != 1 {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("one of iso_file, iso_url, or a combination of cd_files and cd_content must be specified for additional_iso %d", idx))
		}
		if len(c.ISOs[idx].ISOConfig.ISOUrls) == 0 && c.ISOs[idx].ISOConfig.RawSingleISOUrl == "" && c.ISOs[idx].ISODownloadPVE {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("iso_download_pve can only be used together with iso_url"))
		}
	}

	// validate disks
	for idx, disk := range c.Disks {
		switch disk.Type {
		case "ide", "sata", "scsi", "virtio":
		default:
			log.Printf("Disk %d type not set, using default 'scsi'", idx)
			c.Disks[idx].Type = "scsi"
		}
		if disk.DiskFormat == "" {
			log.Printf("Disk %d format not set, using default 'raw'", idx)
			c.Disks[idx].DiskFormat = "raw"
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
		if disk.AsyncIO == "" {
			disk.AsyncIO = "io_uring"
		}
		switch disk.AsyncIO {
		case "native", "threads", "io_uring":
		default:
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("AsyncIO must be native, threads or io_uring"))
		}
		if disk.SSD && disk.Type == "virtio" {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("SSD emulation is not supported on virtio disks"))
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
	if c.CloudInit {
		switch c.CloudInitDiskType {
		case "ide", "scsi", "sata":
		case "":
			log.Printf("Cloud-Init disk type not set, using default 'ide'")
			c.CloudInitDiskType = "ide"
		default:
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("invalid value for `cloud_init_disk_type` %q: only one of 'ide', 'scsi', 'sata' is valid", c.CloudInitDiskType))
		}
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
	if c.TPMConfig != (tpmConfig{}) {
		if c.TPMConfig.TPMStoragePool == "" {
			errs = packersdk.MultiErrorAppend(errs, errors.New("tpm_storage_pool not set for tpm_config"))
		}
		if c.TPMConfig.Version == "" {
			log.Printf("TPM state device defined, but no tpm_version given, using v2.0")
			c.TPMConfig.Version = "v2.0"
		}
		if !(c.TPMConfig.Version == "v1.2" || c.TPMConfig.Version == "v2.0") {
			errs = packersdk.MultiErrorAppend(errs, errors.New("TPM Version must be one of \"v1.2\", \"v2.0\""))
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
