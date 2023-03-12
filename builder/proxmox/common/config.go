// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,NICConfig,diskConfig,vgaConfig,additionalISOsConfig,efiConfig

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

	Boot           string         `mapstructure:"boot"`
	Memory         int            `mapstructure:"memory"`
	BalloonMinimum int            `mapstructure:"ballooning_minimum"`
	Cores          int            `mapstructure:"cores"`
	CPUType        string         `mapstructure:"cpu_type"`
	Sockets        int            `mapstructure:"sockets"`
	Numa           bool           `mapstructure:"numa"`
	OS             string         `mapstructure:"os"`
	BIOS           string         `mapstructure:"bios"`
	EFIConfig      efiConfig      `mapstructure:"efi_config"`
	EFIDisk        string         `mapstructure:"efidisk"`
	Machine        string         `mapstructure:"machine"`
	VGA            vgaConfig      `mapstructure:"vga"`
	NICs           []NICConfig    `mapstructure:"network_adapters"`
	Disks          []diskConfig   `mapstructure:"disks"`
	Serials        []string       `mapstructure:"serials"`
	Agent          config.Trilean `mapstructure:"qemu_agent"`
	SCSIController string         `mapstructure:"scsi_controller"`
	Onboot         bool           `mapstructure:"onboot"`
	DisableKVM     bool           `mapstructure:"disable_kvm"`

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

type NICConfig struct {
	Model        string `mapstructure:"model"`
	PacketQueues int    `mapstructure:"packet_queues"`
	MACAddress   string `mapstructure:"mac_address"`
	MTU          int    `mapstructure:"mtu"`
	Bridge       string `mapstructure:"bridge"`
	VLANTag      string `mapstructure:"vlan_tag"`
	Firewall     bool   `mapstructure:"firewall"`
}
type diskConfig struct {
	Type            string `mapstructure:"type"`
	StoragePool     string `mapstructure:"storage_pool"`
	StoragePoolType string `mapstructure:"storage_pool_type"`
	Size            string `mapstructure:"disk_size"`
	CacheMode       string `mapstructure:"cache_mode"`
	DiskFormat      string `mapstructure:"format"`
	IOThread        bool   `mapstructure:"io_thread"`
}
type efiConfig struct {
	EFIStoragePool  string `mapstructure:"efi_storage_pool"`
	PreEnrolledKeys bool   `mapstructure:"pre_enrolled_keys"`
	EFIType         string `mapstructure:"efi_type"`
}
type vgaConfig struct {
	Type   string `mapstructure:"type"`
	Memory int    `mapstructure:"memory"`
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

	if errs != nil && len(errs.Errors) > 0 {
		return nil, warnings, errs
	}
	return nil, warnings, nil
}
