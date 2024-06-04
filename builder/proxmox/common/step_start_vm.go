// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// stepStartVM takes the given configuration and starts a VM on the given Proxmox node.
//
// It sets the vmRef state which is used throughout the later steps to reference the VM
// in API calls.
type stepStartVM struct {
	vmCreator ProxmoxVMCreator
}

type ProxmoxVMCreator interface {
	Create(*proxmox.VmRef, proxmox.ConfigQemu, multistep.StateBag) error
}
type vmStarter interface {
	CheckVmRef(vmr *proxmox.VmRef) (err error)
	DeleteVm(vmr *proxmox.VmRef) (exitStatus string, err error)
	GetNextID(int) (int, error)
	GetVmConfig(vmr *proxmox.VmRef) (vmConfig map[string]interface{}, err error)
	GetVmRefsByName(vmName string) (vmrs []*proxmox.VmRef, err error)
	SetVmConfig(*proxmox.VmRef, map[string]interface{}) (interface{}, error)
	StartVm(*proxmox.VmRef) (string, error)
}

var (
	maxDuplicateIDRetries = 3
)

// Check if the given builder configuration maps to an existing VM template on the Proxmox cluster.
// Returns an empty *proxmox.VmRef when no matching ID or name is found.
func getExistingTemplate(c *Config, client vmStarter) (*proxmox.VmRef, error) {
	vmRef := &proxmox.VmRef{}
	if c.VMID > 0 {
		log.Printf("looking up VM with ID %d", c.VMID)
		vmRef = proxmox.NewVmRef(c.VMID)
		err := client.CheckVmRef(vmRef)
		if err != nil {
			// expect an error if no VM is found
			// the error string is defined in GetVmInfo() of proxmox-api-go
			notFoundError := fmt.Sprintf("vm '%d' not found", c.VMID)
			if err.Error() == notFoundError {
				log.Println(err.Error())
				return &proxmox.VmRef{}, nil
			}
			return &proxmox.VmRef{}, err
		}
		log.Printf("found VM with ID %d", vmRef.VmId())
	} else {
		log.Printf("looking up VMs with name '%s'", c.TemplateName)
		vmRefs, err := client.GetVmRefsByName(c.TemplateName)
		if err != nil {
			// expect an error if no VMs are found
			// the error string is defined in GetVmRefsByName() of proxmox-api-go
			notFoundError := fmt.Sprintf("vm '%s' not found", c.TemplateName)
			if err.Error() == notFoundError {
				log.Println(err.Error())
				return &proxmox.VmRef{}, nil
			}
			return &proxmox.VmRef{}, err
		}
		if len(vmRefs) > 1 {
			vmIDs := []int{}
			for _, vmr := range vmRefs {
				vmIDs = append(vmIDs, vmr.VmId())
			}
			return &proxmox.VmRef{}, fmt.Errorf("found multiple VMs with name '%s', IDs: %v", c.TemplateName, vmIDs)
		}
		vmRef = vmRefs[0]
		log.Printf("found VM with name '%s' (ID: %d)", c.TemplateName, vmRef.VmId())
	}
	log.Printf("check if VM %d is a template", vmRef.VmId())
	vmConfig, err := client.GetVmConfig(vmRef)
	if err != nil {
		return &proxmox.VmRef{}, err
	}
	log.Printf("VM %d template: %d", vmRef.VmId(), vmConfig["template"])
	if vmConfig["template"] == nil {
		return &proxmox.VmRef{}, fmt.Errorf("found matching VM (ID: %d, name: %s), but it is not a template", vmRef.VmId(), vmConfig["name"])
	}
	return vmRef, nil
}

func (s *stepStartVM) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(vmStarter)
	c := state.Get("config").(*Config)

	agent := 1
	if c.Agent.False() {
		agent = 0
	}

	kvm := true
	if c.DisableKVM {
		kvm = false
	}

	config := proxmox.ConfigQemu{
		Name:           c.VMName,
		Agent:          agent,
		QemuKVM:        &kvm,
		Tags:           c.Tags,
		Boot:           c.Boot, // Boot priority, example: "order=virtio0;ide2;net0", virtio0:Disk0 -> ide0:CDROM -> net0:Network
		QemuCpu:        c.CPUType,
		Description:    "Packer ephemeral build VM",
		Memory:         c.Memory,
		QemuCores:      c.Cores,
		QemuSockets:    c.Sockets,
		QemuNuma:       &c.Numa,
		QemuOs:         c.OS,
		Bios:           c.BIOS,
		EFIDisk:        generateProxmoxEfi(c.EFIConfig),
		Machine:        c.Machine,
		RNGDrive:       generateProxmoxRng0(c.Rng0),
		TPM:            generateProxmoxTpm(c.TPMConfig),
		QemuVga:        generateProxmoxVga(c.VGA),
		QemuNetworks:   generateProxmoxNetworkAdapters(c.NICs),
		Disks:          generateProxmoxDisks(c.Disks, c.AdditionalISOFiles, c.ISOBuilderCDROMDevice),
		QemuPCIDevices: generateProxmoxPCIDeviceMap(c.PCIDevices),
		QemuSerials:    generateProxmoxSerials(c.Serials),
		Scsihw:         c.SCSIController,
		Onboot:         &c.Onboot,
		Args:           c.AdditionalArgs,
	}

	// 0 disables the ballooning device, which is useful for all VMs
	// and should be kept enabled by default.
	// See https://github.com/hashicorp/packer-plugin-proxmox/issues/127#issuecomment-1464030102
	if c.BalloonMinimum > 0 {
		config.Balloon = c.BalloonMinimum
	}

	if c.PackerForce {
		ui.Say("Force set, checking for existing artifact on PVE cluster")
		vmRef, err := getExistingTemplate(c, client)
		if err != nil {
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		if vmRef.VmId() != 0 {
			ui.Say(fmt.Sprintf("found existing VM template with ID %d on PVE node %s, deleting it", vmRef.VmId(), vmRef.Node()))
			_, err = client.DeleteVm(vmRef)
			if err != nil {
				state.Put("error", err)
				ui.Error(fmt.Sprintf("error deleting VM template: %s", err.Error()))
				return multistep.ActionHalt
			}
			ui.Say(fmt.Sprintf("Successfully deleted VM template %d", vmRef.VmId()))
		} else {
			ui.Say("No existing artifact found")
		}
	}

	ui.Say("Creating VM")
	var vmRef *proxmox.VmRef
	for i := 1; ; i++ {
		id := c.VMID
		if id == 0 {
			ui.Say("No VM ID given, getting next free from Proxmox")
			genID, err := client.GetNextID(0)
			if err != nil {
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
			id = genID
			config.VmID = genID
		}
		vmRef = proxmox.NewVmRef(id)
		vmRef.SetNode(c.Node)
		if c.Pool != "" {
			vmRef.SetPool(c.Pool)
			config.Pool = c.Pool
		}

		err := s.vmCreator.Create(vmRef, config, state)
		if err == nil {
			break
		}

		// If there's no explicitly configured VMID, and the error is caused
		// by a race condition in someone else using the ID we just got
		// generated, we'll retry up to maxDuplicateIDRetries times.
		if c.VMID == 0 && isDuplicateIDError(err) && i < maxDuplicateIDRetries {
			ui.Say("Generated VM ID was already allocated, retrying")
			continue
		}
		err = fmt.Errorf("Error creating VM: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// The EFI disk doesn't get created reliably when using the clone builder,
	// so let's make sure it's there.
	if c.EFIConfig != (efiConfig{}) && c.Ctx.BuildType == "proxmox-clone" {
		addEFIConfig := make(map[string]interface{})
		config.CreateQemuEfiParams(addEFIConfig)
		_, err := client.SetVmConfig(vmRef, addEFIConfig)
		if err != nil {
			err := fmt.Errorf("error updating template: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	// Store the vm id for later
	state.Put("vmRef", vmRef)
	// instance_id is the generic term used so that users can have access to the
	// instance id inside of the provisioners, used in step_provision.
	// Note that this is just the VMID, we do not keep the node, pool and other
	// info available in the vmref type.
	state.Put("instance_id", vmRef.VmId())

	ui.Say("Starting VM")
	_, err := client.StartVm(vmRef)
	if err != nil {
		err := fmt.Errorf("Error starting VM: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func generateProxmoxNetworkAdapters(nics []NICConfig) proxmox.QemuDevices {
	devs := make(proxmox.QemuDevices)
	for idx := range nics {
		devs[idx] = make(proxmox.QemuDevice)
		setDeviceParamIfDefined(devs[idx], "model", nics[idx].Model)
		setDeviceParamIfDefined(devs[idx], "macaddr", nics[idx].MACAddress)
		setDeviceParamIfDefined(devs[idx], "bridge", nics[idx].Bridge)
		setDeviceParamIfDefined(devs[idx], "tag", nics[idx].VLANTag)

		if nics[idx].Firewall {
			devs[idx]["firewall"] = nics[idx].Firewall
		}

		if nics[idx].MTU > 0 {
			devs[idx]["mtu"] = nics[idx].MTU
		}
		if nics[idx].PacketQueues > 0 {
			devs[idx]["queues"] = nics[idx].PacketQueues
		}
	}
	return devs
}

func generateProxmoxDisks(disks []diskConfig, additionalISOFiles []additionalISOsConfig, bootiso string) *proxmox.QemuStorages {
	ideDisks := proxmox.QemuIdeDisks{}
	sataDisks := proxmox.QemuSataDisks{}
	scsiDisks := proxmox.QemuScsiDisks{}
	virtIODisks := proxmox.QemuVirtIODisks{}

	// additionalISOsConfig accepts a static device type and index value in Device.
	// Disks accept a device type but no index value.
	//
	// If this is a proxmox-iso build, the boot iso device is mapped after this function (builder/proxmox/iso/builder.go func Create)
	// Map Additional ISO files first to ensure they get their assigned device index then proceed with disks in remaining available fields.
	if len(additionalISOFiles) > 0 {
		for _, iso := range additionalISOFiles {
			// IsoFile struct parses the ISO File and Storage Pool as separate fields.
			isoFile := strings.Split(iso.ISOFile, ":iso/")

			// define QemuCdRom containing isoFile properties
			bootIso := &proxmox.QemuCdRom{
				Iso: &proxmox.IsoFile{
					File:    isoFile[1],
					Storage: isoFile[0],
				},
			}

			// extract device type from ISODevice config value eg. ide from ide2
			// validation of the iso.Device value occurs in builder/proxmox/common/config.go func Prepare
			rd := regexp.MustCompile(`\D+`)
			device := rd.FindString(iso.Device)
			// extract device index from ISODevice config value eg. 2 from ide2
			rb := regexp.MustCompile(`\d+`)
			index, _ := strconv.Atoi(rb.FindString(iso.Device))

			log.Printf("Mapping Additional ISO to %s%d", device, index)
			switch device {
			case "ide":
				dev := proxmox.QemuIdeStorage{
					CdRom: bootIso,
				}
				reflect.
					ValueOf(&ideDisks).Elem().
					FieldByName(fmt.Sprintf("Disk_%d", index)).
					Set(reflect.ValueOf(&dev))
			case "sata":
				dev := proxmox.QemuSataStorage{
					CdRom: bootIso,
				}
				reflect.
					ValueOf(&sataDisks).Elem().
					FieldByName(fmt.Sprintf("Disk_%d", index)).
					Set(reflect.ValueOf(&dev))
			case "scsi":
				dev := proxmox.QemuScsiStorage{
					CdRom: bootIso,
				}
				reflect.ValueOf(&scsiDisks).Elem().
					FieldByName(fmt.Sprintf("Disk_%d", index)).
					Set(reflect.ValueOf(&dev))
			}
		}
	}

	ideCount := 0
	sataCount := 0
	scsiCount := 0
	virtIOCount := 0

	// Map Disks
	for idx := range disks {
		tmpSize, _ := strconv.ParseInt(disks[idx].Size[:len(disks[idx].Size)-1], 10, 0)
		size := proxmox.QemuDiskSize(0)
		switch disks[idx].Size[len(disks[idx].Size)-1:] {
		case "T":
			size = proxmox.QemuDiskSize(tmpSize) * 1073741824
		case "G":
			size = proxmox.QemuDiskSize(tmpSize) * 1048576
		case "M":
			size = proxmox.QemuDiskSize(tmpSize) * 1024
		case "K":
			size = proxmox.QemuDiskSize(tmpSize)
		}

		switch disks[idx].Type {
		case "ide":
			dev := proxmox.QemuIdeStorage{
				Disk: &proxmox.QemuIdeDisk{
					SizeInKibibytes: size,
					Storage:         disks[idx].StoragePool,
					AsyncIO:         proxmox.QemuDiskAsyncIO(disks[idx].AsyncIO),
					Cache:           proxmox.QemuDiskCache(disks[idx].CacheMode),
					Format:          proxmox.QemuDiskFormat(disks[idx].DiskFormat),
					Discard:         disks[idx].Discard,
					EmulateSSD:      disks[idx].SSD,
				},
			}
			for {
				log.Printf("Mapping Disk to ide%d", ideCount)
				// to avoid a panic if IDE has too many devices attached, exit the loop when all indexes are occupied
				if ideCount > 3 {
					log.Print("No further IDE device indexes available (Max 4 devices).")
					break
				}
				// We need reflection here as the storage objects are not exposed
				// as a slice, but as a series of named fields in the structure
				// that the APIs use.
				//
				// This means that assigning the disks in the order they're defined
				// in would result in a bunch of `switch` cases for the index, and
				// named field assignation for each.
				//
				// Example:
				// ```
				// switch ideCount {
				// case 0:
				//	dev.Disk_0 = dev
				// case 1:
				//	dev.Disk_1 = dev
				// [...]
				// }
				// ```
				//
				// Instead, we use reflection to address the fields algorithmically,
				// so we don't need to write this verbose code.
				if reflect.
					// We need to get the pointer to the structure so we can
					// assign a value to the disk
					ValueOf(&ideDisks).Elem().
					// Get the field from its name, each disk's field has a
					// similar format 'Disk_%d'
					FieldByName(fmt.Sprintf("Disk_%d", ideCount)).
					// Return if the field has no device already attached to it
					// and not proxmox-iso's configured boot iso device index
					IsNil() && bootiso != fmt.Sprintf("ide%d", ideCount) {
					reflect.
						ValueOf(&ideDisks).Elem().
						FieldByName(fmt.Sprintf("Disk_%d", ideCount)).
						// Assign dev to the Disk_%d field
						Set(reflect.ValueOf(&dev))
					ideCount++
					break
				}
				// if the disk field is not empty (occupied by an ISO), try the next index
				log.Printf("ide%d occupied, trying next device index", ideCount)
				ideCount++
			}
		case "scsi":
			dev := proxmox.QemuScsiStorage{
				Disk: &proxmox.QemuScsiDisk{
					SizeInKibibytes: size,
					Storage:         disks[idx].StoragePool,
					AsyncIO:         proxmox.QemuDiskAsyncIO(disks[idx].AsyncIO),
					Cache:           proxmox.QemuDiskCache(disks[idx].CacheMode),
					Format:          proxmox.QemuDiskFormat(disks[idx].DiskFormat),
					Discard:         disks[idx].Discard,
					EmulateSSD:      disks[idx].SSD,
					IOThread:        disks[idx].IOThread,
				},
			}
			for {
				log.Printf("Mapping Disk to scsi%d", scsiCount)
				if scsiCount > 30 {
					log.Print("No further SCSI device indexes available (Max 31 devices).")
					break
				}
				if reflect.
					ValueOf(&scsiDisks).Elem().
					FieldByName(fmt.Sprintf("Disk_%d", scsiCount)).
					IsNil() && bootiso != fmt.Sprintf("scsi%d", scsiCount) {
					reflect.
						ValueOf(&scsiDisks).Elem().
						FieldByName(fmt.Sprintf("Disk_%d", scsiCount)).
						Set(reflect.ValueOf(&dev))
					scsiCount++
					break
				}
				log.Printf("scsi%d occupied, trying next device index", scsiCount)
				scsiCount++
			}
		case "sata":
			dev := proxmox.QemuSataStorage{
				Disk: &proxmox.QemuSataDisk{
					SizeInKibibytes: size,
					Storage:         disks[idx].StoragePool,
					AsyncIO:         proxmox.QemuDiskAsyncIO(disks[idx].AsyncIO),
					Cache:           proxmox.QemuDiskCache(disks[idx].CacheMode),
					Format:          proxmox.QemuDiskFormat(disks[idx].DiskFormat),
					Discard:         disks[idx].Discard,
					EmulateSSD:      disks[idx].SSD,
				},
			}
			for {
				if sataCount > 5 {
					log.Print("No further SATA device indexes available (Max 6 devices).")
					break
				}
				log.Printf("Mapping Disk to sata%d", sataCount)
				if reflect.
					ValueOf(&sataDisks).Elem().
					FieldByName(fmt.Sprintf("Disk_%d", sataCount)).
					IsNil() && bootiso != fmt.Sprintf("sata%d", sataCount) {
					reflect.
						ValueOf(&sataDisks).Elem().
						FieldByName(fmt.Sprintf("Disk_%d", sataCount)).
						Set(reflect.ValueOf(&dev))
					sataCount++
					break
				}
				log.Printf("sata%d occupied, trying next device index", sataCount)
				sataCount++
			}
		case "virtio":
			dev := proxmox.QemuVirtIOStorage{
				Disk: &proxmox.QemuVirtIODisk{
					SizeInKibibytes: size,
					Storage:         disks[idx].StoragePool,
					AsyncIO:         proxmox.QemuDiskAsyncIO(disks[idx].AsyncIO),
					Cache:           proxmox.QemuDiskCache(disks[idx].CacheMode),
					Format:          proxmox.QemuDiskFormat(disks[idx].DiskFormat),
					Discard:         disks[idx].Discard,
					IOThread:        disks[idx].IOThread,
				},
			}
			for {
				log.Printf("Mapping Disk to virtio%d", virtIOCount)
				if virtIOCount > 15 {
					log.Print("No further VirtIO device indexes available (Max 16 devices).")
					break
				}
				if reflect.
					ValueOf(&virtIODisks).Elem().
					FieldByName(fmt.Sprintf("Disk_%d", virtIOCount)).
					IsNil() {
					reflect.
						ValueOf(&virtIODisks).Elem().
						FieldByName(fmt.Sprintf("Disk_%d", virtIOCount)).
						Set(reflect.ValueOf(&dev))
					virtIOCount++
					break
				}
				log.Printf("virtio%d occupied, trying next device index", virtIOCount)
				virtIOCount++
			}
		}
	}
	return &proxmox.QemuStorages{
		Ide:    &ideDisks,
		Sata:   &sataDisks,
		Scsi:   &scsiDisks,
		VirtIO: &virtIODisks,
	}
}

func generateProxmoxPCIDeviceMap(devices []pciDeviceConfig) proxmox.QemuDevices {
	devs := make(proxmox.QemuDevices)
	for idx := range devices {
		devs[idx] = make(proxmox.QemuDevice)
		setDeviceParamIfDefined(devs[idx], "host", devices[idx].Host)
		setDeviceParamIfDefined(devs[idx], "device-id", devices[idx].DeviceID)
		setDeviceParamIfDefined(devs[idx], "mapping", devices[idx].Mapping)
		setDeviceParamIfDefined(devs[idx], "mdev", devices[idx].MDEV)
		setDeviceParamIfDefined(devs[idx], "romfile", devices[idx].ROMFile)
		setDeviceParamIfDefined(devs[idx], "sub-device-id", devices[idx].SubDeviceID)
		setDeviceParamIfDefined(devs[idx], "sub-vendor-id", devices[idx].SubVendorID)
		setDeviceParamIfDefined(devs[idx], "vendor-id", devices[idx].VendorID)

		devs[idx]["pcie"] = strconv.FormatBool(devices[idx].PCIe)
		devs[idx]["rombar"] = strconv.FormatBool(!devices[idx].HideROMBAR)
		devs[idx]["x-vga"] = strconv.FormatBool(devices[idx].XVGA)
		devs[idx]["legacy-igd"] = strconv.FormatBool(devices[idx].LegacyIGD)
	}
	return devs
}

func generateProxmoxSerials(serials []string) proxmox.QemuDevices {
	devs := make(proxmox.QemuDevices)
	for idx := range serials {
		devs[idx] = make(proxmox.QemuDevice)
		setDeviceParamIfDefined(devs[idx], "type", serials[idx])
	}
	return devs
}

func generateProxmoxRng0(rng0 rng0Config) proxmox.QemuDevice {
	dev := make(proxmox.QemuDevice)
	setDeviceParamIfDefined(dev, "source", rng0.Source)

	if rng0.MaxBytes >= 0 {
		dev["max_bytes"] = rng0.MaxBytes
	}
	if rng0.Period > 0 {
		dev["period"] = rng0.Period
	}
	return dev
}

func generateProxmoxVga(vga vgaConfig) proxmox.QemuDevice {
	dev := make(proxmox.QemuDevice)
	setDeviceParamIfDefined(dev, "type", vga.Type)

	if vga.Memory > 0 {
		dev["memory"] = vga.Memory
	}
	return dev
}

func generateProxmoxEfi(efi efiConfig) proxmox.QemuDevice {
	dev := make(proxmox.QemuDevice)
	setDeviceParamIfDefined(dev, "storage", efi.EFIStoragePool)
	setDeviceParamIfDefined(dev, "efitype", efi.EFIType)
	// efi.PreEnrolledKeys can be false, but we only want to set pre-enrolled-keys=0
	// when other EFI options are set.
	if len(dev) > 0 {
		if efi.PreEnrolledKeys {
			dev["pre-enrolled-keys"] = "1"
		} else {
			dev["pre-enrolled-keys"] = "0"
		}
	}
	return dev
}

func generateProxmoxTpm(tpm tpmConfig) *proxmox.TpmState {
	// If no TPM config is presented, don't return a TpmState device
	if tpm == (tpmConfig{}) {
		return nil
	}

	dev := proxmox.TpmState{
		Storage: tpm.TPMStoragePool,
		Version: (*proxmox.TpmVersion)(&tpm.Version),
	}
	return &dev
}

func setDeviceParamIfDefined(dev proxmox.QemuDevice, key, value string) {
	// Empty string is considered as not defined
	if value != "" {
		dev[key] = value
	}
}

func isDuplicateIDError(err error) bool {
	return strings.Contains(err.Error(), "already exists on node")
}

type startedVMCleaner interface {
	StopVm(*proxmox.VmRef) (string, error)
	DeleteVm(*proxmox.VmRef) (string, error)
}

var _ startedVMCleaner = &proxmox.Client{}

func (s *stepStartVM) Cleanup(state multistep.StateBag) {
	vmRefUntyped, ok := state.GetOk("vmRef")
	// If not ok, we probably errored out before creating the VM
	if !ok {
		return
	}
	vmRef := vmRefUntyped.(*proxmox.VmRef)

	// The vmRef will actually refer to the created template if everything
	// finished successfully, so in that case we shouldn't cleanup
	if _, ok := state.GetOk("success"); ok {
		return
	}

	client := state.Get("proxmoxClient").(startedVMCleaner)
	ui := state.Get("ui").(packersdk.Ui)

	// Destroy the server we just created
	ui.Say("Stopping VM")
	_, err := client.StopVm(vmRef)
	if err != nil {
		ui.Error(fmt.Sprintf("Error stopping VM. Please stop and delete it manually: %s", err))
		return
	}

	ui.Say("Deleting VM")
	_, err = client.DeleteVm(vmRef)
	if err != nil {
		ui.Error(fmt.Sprintf("Error deleting VM. Please delete it manually: %s", err))
		return
	}
}
