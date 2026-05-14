// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"fmt"
	"log"
	"net"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

// stepStartVM takes the given configuration and starts a VM on the given Proxmox node.
//
// It sets the vmRef state which is used throughout the later steps to reference the VM
// in API calls.
type stepStartVM struct {
	vmCreator ProxmoxVMCreator
}

type ProxmoxVMCreator interface {
	Create(context.Context, proxmox.ConfigQemu, multistep.StateBag) (*proxmox.VmRef, error)
}
type vmStarter interface {
	CheckVmRef(ctx context.Context, vmr *proxmox.VmRef) (err error)
	DeleteVm(ctx context.Context, vmr *proxmox.VmRef) (exitStatus string, err error)
	GetNextID(ctx context.Context, startID *proxmox.GuestID) (proxmox.GuestID, error)
	GetVmConfig(ctx context.Context, vmr *proxmox.VmRef) (vmConfig map[string]interface{}, err error)
	GetVmRefsByName(ctx context.Context, vmName proxmox.GuestName) (vmrs []*proxmox.VmRef, err error)
	SetVmConfig(*proxmox.VmRef, map[string]interface{}) (interface{}, error)
	StartVm(ctx context.Context, vmr *proxmox.VmRef) (string, error)
}

var (
	maxDuplicateIDRetries = 3
)

// Check if the given builder configuration maps to an existing VM template on the Proxmox cluster.
// Returns an empty *proxmox.VmRef when no matching ID or name is found.
func getExistingTemplate(ctx context.Context, c *Config, client vmStarter) (*proxmox.VmRef, error) {
	vmRef := &proxmox.VmRef{}
	if c.VMID > 0 {
		log.Printf("looking up VM with ID %d", c.VMID)
		vmRef = proxmox.NewVmRef(proxmox.GuestID(c.VMID))
		err := client.CheckVmRef(ctx, vmRef)
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
		vmRefs, err := client.GetVmRefsByName(ctx, proxmox.GuestName(c.TemplateName))
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
			vmIDs := []proxmox.GuestID{}
			for _, vmr := range vmRefs {
				vmIDs = append(vmIDs, vmr.VmId())
			}
			return &proxmox.VmRef{}, fmt.Errorf("found multiple VMs with name '%s', IDs: %v", c.TemplateName, vmIDs)
		}
		vmRef = vmRefs[0]
		log.Printf("found VM with name '%s' (ID: %d)", c.TemplateName, vmRef.VmId())
	}
	log.Printf("check if VM %d is a template", vmRef.VmId())
	vmConfig, err := client.GetVmConfig(ctx, vmRef)
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

	kvm := true
	if c.DisableKVM {
		kvm = false
	}

	errs, warnings, disks := generateProxmoxDisks(c.Disks, c.ISOs, c.CloneSourceDisks)
	if errs != nil && len(errs.Errors) > 0 {
		state.Put("error", errs)
		ui.Error(errs.Error())
		return multistep.ActionHalt
	}
	if len(warnings) > 0 {
		for idx := range warnings {
			ui.Sayf("Warning: %s", warnings[idx])
		}
	}

	var description = "Packer ephemeral build VM"
	name := proxmox.GuestName(c.VMName)
	pool := proxmox.PoolName(c.Pool)

	config := proxmox.ConfigQemu{
		Name:    &name,
		Agent:   generateAgentConfig(c.Agent),
		QemuKVM: &kvm,
		Tags:    generateTags(c.Tags),
		Boot:    c.Boot, // Boot priority, example: "order=virtio0;ide2;net0", virtio0:Disk0 -> ide0:CDROM -> net0:Network
		CPU: &proxmox.QemuCPU{
			Cores:   (*proxmox.QemuCpuCores)(&c.Cores),
			Sockets: (*proxmox.QemuCpuSockets)(&c.Sockets),
			Numa:    &c.Numa,
			Type:    (*proxmox.CpuType)(&c.CPUType),
		},
		Description: &description,
		Memory: &proxmox.QemuMemory{
			CapacityMiB: (*proxmox.QemuMemoryCapacity)(&c.Memory),
		},
		QemuOs:           c.OS,
		CreateOptions:    generateProxmoxCreateOptions(c.Arch),
		Bios:             c.BIOS,
		EfiDisk:          generateProxmoxEfi(c.EFIConfig),
		Machine:          c.Machine,
		RandomnessDevice: generateProxmoxRng0(c.Rng0),
		TPM:              generateProxmoxTpm(c.TPMConfig),
		QemuVga:          generateProxmoxVga(c.VGA),
		Networks:         generateProxmoxNetworkAdapters(c.NICs),
		Disks:            disks,
		PciDevices:       generateProxmoxPCIDeviceMap(c.PCIDevices),
		Serials:          generateProxmoxSerials(c.Serials),
		Scsihw:           c.SCSIController,
		StartAtNodeBoot:  &c.Onboot,
		Args:             c.AdditionalArgs,
		Pool:             &pool,
	}

	// 0 disables the ballooning device, which is useful for all VMs
	// and should be kept enabled by default.
	// See https://github.com/hashicorp/packer-plugin-proxmox/issues/127#issuecomment-1464030102
	if c.BalloonMinimum > 0 {
		config.Memory.MinimumCapacityMiB = (*proxmox.QemuMemoryBalloonCapacity)(&c.BalloonMinimum)
	}

	if c.PackerForce {
		ui.Say("Force set, checking for existing artifact on PVE cluster")
		vmRef, err := getExistingTemplate(ctx, c, client)
		if err != nil {
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		if vmRef.VmId() != 0 {
			ui.Say(fmt.Sprintf("found existing VM template with ID %d on PVE node %s, deleting it", vmRef.VmId(), vmRef.Node()))
			_, err = client.DeleteVm(ctx, vmRef)
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
	node := proxmox.NodeName(c.Node)
	config.Node = &node
	var vmRef *proxmox.VmRef
	for i := 1; ; i++ {
		if c.VMID > 0 {
			id := proxmox.GuestID(c.VMID)
			config.ID = &id
		} else {
			ui.Say("No VM ID given, getting next free from Proxmox")
			genID, err := client.GetNextID(ctx, nil)
			if err != nil {
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
			config.ID = &genID
		}

		var err error
		vmRef, err = s.vmCreator.Create(ctx, config, state)
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

	// Store the vm id for later
	state.Put("vmRef", vmRef)
	// instance_id is the generic term used so that users can have access to the
	// instance id inside of the provisioners, used in step_provision.
	// Note that this is just the VMID, we do not keep the node, pool and other
	// info available in the vmref type.
	state.Put("instance_id", int(vmRef.VmId()))

	ui.Say("Starting VM")
	_, err := client.StartVm(ctx, vmRef)
	if err != nil {
		err := fmt.Errorf("Error starting VM: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func generateAgentConfig(agent config.Trilean) *proxmox.QemuGuestAgent {
	var enableAgent bool

	if agent.True() {
		enableAgent = true
	}

	return &proxmox.QemuGuestAgent{
		Enable: &enableAgent,
	}
}

func generateTags(rawTags string) *proxmox.Tags {
	tags := proxmox.Tags{}
	if rawTags == "" {
		return &tags
	}
	for _, tag := range strings.Split(rawTags, ";") {
		tags = append(tags, proxmox.Tag(tag))
	}
	return &tags
}

func generateProxmoxNetworkAdapters(nics []NICConfig) proxmox.QemuNetworkInterfaces {
	out := make(proxmox.QemuNetworkInterfaces, len(nics))
	for idx, n := range nics {
		iface := proxmox.QemuNetworkInterface{}
		if n.Model != "" {
			model := proxmox.QemuNetworkModel(n.Model)
			iface.Model = &model
		}
		if n.MACAddress != "" {
			if hw, err := net.ParseMAC(n.MACAddress); err == nil {
				iface.MAC = &hw
			}
		}
		if n.Bridge != "" {
			bridge := n.Bridge
			iface.Bridge = &bridge
		}
		if n.VLANTag != "" {
			if v, err := strconv.Atoi(n.VLANTag); err == nil {
				vlan := proxmox.Vlan(v)
				iface.NativeVlan = &vlan
			}
		}
		if n.Firewall {
			fw := true
			iface.Firewall = &fw
		}
		if n.MTU > 0 {
			iface.MTU = &proxmox.QemuMTU{Value: proxmox.MTU(n.MTU)}
		}
		if n.PacketQueues > 0 {
			q := proxmox.QemuNetworkQueue(n.PacketQueues)
			iface.MultiQueue = &q
		}
		out[proxmox.QemuNetworkInterfaceID(idx)] = iface
	}
	return out
}

func generateProxmoxDisks(disks []diskConfig, isos []ISOsConfig, cloneSourceDisks []string) (*packersdk.MultiError, []string, *proxmox.QemuStorages) {
	ideDisks := proxmox.QemuIdeDisks{}
	sataDisks := proxmox.QemuSataDisks{}
	scsiDisks := proxmox.QemuScsiDisks{}
	virtIODisks := proxmox.QemuVirtIODisks{}
	qemuStorages := proxmox.QemuStorages{
		Ide:    &ideDisks,
		Sata:   &sataDisks,
		Scsi:   &scsiDisks,
		VirtIO: &virtIODisks,
	}

	ideCount := 0
	sataCount := 0
	scsiCount := 0
	virtIOCount := 0

	var errs *packersdk.MultiError
	var warnings []string

	// Versions up to 1.8 supported static assignment of ISOs to a bus index, however hard disks did not support static bus indexes.
	// For backwards compatibility handle statically mapped ISOs first (guarantee allocation) then allocate hard disks and remaining ISOs in free slots around them.

	// Map ISOs with a static index assignment
	if len(isos) > 0 {
		for idx := range isos {
			// if a static assignment has been defined
			if isos[idx].Index != "" {
				// IsoFile struct parses the ISO File and Storage Pool as separate fields.
				isoFile := strings.Split(isos[idx].ISOFile, ":iso/")

				// define QemuCdRom containing isoFile properties
				cdrom := &proxmox.QemuCdRom{
					Iso: &proxmox.IsoFile{
						File:    isoFile[1],
						Storage: isoFile[0],
					},
				}

				switch isos[idx].Type {
				case "ide":
					dev := proxmox.QemuIdeStorage{
						CdRom: cdrom,
					}
					deviceIndex := fmt.Sprintf("ide%s", isos[idx].Index)
					log.Printf("Mapping static assigned ISO to %s", deviceIndex)
					if slices.Contains(cloneSourceDisks, deviceIndex) {
						// Backwards compatibility: statically assigned ISOs overwrote assignments of existing disks when using the clone builder
						// issue a warning so users are aware and can decide if they want to remap the ISO device
						warnings = append(warnings, fmt.Sprintf("an existing hard disk was found at %s on the clone source VM, overwriting with ISO configured for the same address", deviceIndex))
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
					reflect.
						// We need to get the pointer to the structure so we can
						// assign a value to the disk
						ValueOf(&ideDisks).Elem().
						// Get the field from its name, each disk's field has a
						// similar format 'Disk_%d'
						FieldByName(fmt.Sprintf("Disk_%s", isos[idx].Index)).
						// Assign dev to the Disk_%d field
						Set(reflect.ValueOf(&dev))
					isos[idx].AssignedDeviceIndex = deviceIndex
				case "sata":
					dev := proxmox.QemuSataStorage{
						CdRom: cdrom,
					}
					deviceIndex := fmt.Sprintf("sata%s", isos[idx].Index)
					log.Printf("Mapping static assigned ISO to %s", deviceIndex)
					if slices.Contains(cloneSourceDisks, deviceIndex) {
						warnings = append(warnings, fmt.Sprintf("an existing hard disk was found at %s on the clone source VM, overwriting with ISO configured for the same address", deviceIndex))
					}
					reflect.
						ValueOf(&sataDisks).Elem().
						FieldByName(fmt.Sprintf("Disk_%s", isos[idx].Index)).
						Set(reflect.ValueOf(&dev))
					isos[idx].AssignedDeviceIndex = deviceIndex
				case "scsi":
					dev := proxmox.QemuScsiStorage{
						CdRom: cdrom,
					}
					deviceIndex := fmt.Sprintf("scsi%s", isos[idx].Index)
					log.Printf("Mapping static assigned ISO to %s", deviceIndex)
					if slices.Contains(cloneSourceDisks, fmt.Sprintf("scsi%d", scsiCount)) {
						warnings = append(warnings, fmt.Sprintf("an existing hard disk was found at %s on the clone source VM, overwriting with ISO configured for the same address", deviceIndex))
					}
					reflect.
						ValueOf(&scsiDisks).Elem().
						FieldByName(fmt.Sprintf("Disk_%s", isos[idx].Index)).
						Set(reflect.ValueOf(&dev))
					isos[idx].AssignedDeviceIndex = deviceIndex
				}
			}
		}
	}

	// Map Hard Disks
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
		backup := true
		if disks[idx].ExcludeFromBackup {
			backup = false
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
					Backup:          backup,
				},
			}
			for {
				log.Printf("Mapping Disk to ide%d", ideCount)
				// If IDE has too many devices attached pass an error then exit the loop
				if ideCount > 3 {
					errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("storage enumeration reached ide index %d, too many ide devices configured. Ensure total Disk and ISO ide assignments don't exceed 4 devices", ideCount))
					break
				}

				// If this ide device index isn't occupied by a disk on a clone builder source vm
				if !slices.Contains(cloneSourceDisks, fmt.Sprintf("ide%d", ideCount)) &&
					// or occupied by a statically mapped ISO
					reflect.
						ValueOf(&ideDisks).Elem().
						FieldByName(fmt.Sprintf("Disk_%d", ideCount)).
						IsNil() {
					reflect.
						ValueOf(&ideDisks).Elem().
						FieldByName(fmt.Sprintf("Disk_%d", ideCount)).
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
					Backup:          backup,
				},
			}
			for {
				log.Printf("Mapping Disk to scsi%d", scsiCount)
				if scsiCount > 30 {
					errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("storage enumeration reached scsi index %d, too many scsi devices configured. Ensure total disk and ISO scsi assignments don't exceed 31 devices", scsiCount))
					break
				}
				if !slices.Contains(cloneSourceDisks, fmt.Sprintf("scsi%d", scsiCount)) &&
					reflect.ValueOf(&scsiDisks).Elem().
						FieldByName(fmt.Sprintf("Disk_%d", scsiCount)).
						IsNil() {
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
					Backup:          backup,
				},
			}
			for {
				if sataCount > 5 {
					errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("storage enumeration reached sata index %d, too many sata devices configured. Ensure total disk and ISO sata assignments don't exceed 6 devices", sataCount))
					break
				}
				log.Printf("Mapping Disk to sata%d", sataCount)
				if !slices.Contains(cloneSourceDisks, fmt.Sprintf("sata%d", sataCount)) &&
					reflect.ValueOf(&sataDisks).Elem().
						FieldByName(fmt.Sprintf("Disk_%d", sataCount)).
						IsNil() {
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
					Backup:          backup,
				},
			}
			for {
				log.Printf("Mapping Disk to virtio%d", virtIOCount)
				if virtIOCount > 15 {
					errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("enumeration reached virtio index %d, too many virtio devices configured. Ensure total disk and ISO virtio assignments don't exceed 16 devices", virtIOCount))
					break
				}
				if !slices.Contains(cloneSourceDisks, fmt.Sprintf("virtio%d", virtIOCount)) &&
					reflect.ValueOf(&virtIODisks).Elem().
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

	// Map ISOs without static mappings in remaining device indexes
	if len(isos) > 0 {
		for idx := range isos {
			// if a static assignment has not been defined
			if isos[idx].Index == "" {
				// IsoFile struct parses the ISO File and Storage Pool as separate fields.
				isoFile := strings.Split(isos[idx].ISOFile, ":iso/")

				// define QemuCdRom containing isoFile properties
				cdrom := &proxmox.QemuCdRom{
					Iso: &proxmox.IsoFile{
						File:    isoFile[1],
						Storage: isoFile[0],
					},
				}

				switch isos[idx].Type {
				case "ide":
					dev := proxmox.QemuIdeStorage{
						CdRom: cdrom,
					}
					for {
						log.Printf("Mapping ISO to ide%d", ideCount)
						if ideCount > 3 {
							errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("storage enumeration reached ide index %d, too many ide devices configured. Ensure total Disk and ISO ide assignments don't exceed 4 devices", ideCount))
							break
						}
						if !slices.Contains(cloneSourceDisks, fmt.Sprintf("ide%d", ideCount)) &&
							reflect.ValueOf(&ideDisks).Elem().
								FieldByName(fmt.Sprintf("Disk_%d", ideCount)).
								IsNil() {
							reflect.
								ValueOf(&ideDisks).Elem().
								FieldByName(fmt.Sprintf("Disk_%d", ideCount)).
								Set(reflect.ValueOf(&dev))
							isos[idx].AssignedDeviceIndex = fmt.Sprintf("ide%d", ideCount)
							ideCount++
							break
						}
						log.Printf("ide%d occupied, trying next device index", ideCount)
						ideCount++
					}
				case "sata":
					dev := proxmox.QemuSataStorage{
						CdRom: cdrom,
					}
					for {
						if sataCount > 5 {
							errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("storage enumeration reached sata index %d, too many sata devices configured. Ensure total disk and ISO sata assignments don't exceed 6 devices", sataCount))
							break
						}
						log.Printf("Mapping ISO to sata%d", sataCount)
						if !slices.Contains(cloneSourceDisks, fmt.Sprintf("sata%d", sataCount)) &&
							reflect.ValueOf(&sataDisks).Elem().
								FieldByName(fmt.Sprintf("Disk_%d", sataCount)).
								IsNil() {
							reflect.
								ValueOf(&sataDisks).Elem().
								FieldByName(fmt.Sprintf("Disk_%d", sataCount)).
								Set(reflect.ValueOf(&dev))
							isos[idx].AssignedDeviceIndex = fmt.Sprintf("sata%d", sataCount)
							sataCount++
							break
						}
						log.Printf("sata%d occupied, trying next device index", sataCount)
						sataCount++
					}
				case "scsi":
					dev := proxmox.QemuScsiStorage{
						CdRom: cdrom,
					}
					for {
						log.Printf("Mapping ISO to scsi%d", scsiCount)
						if scsiCount > 30 {
							errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("storage enumeration reached scsi index %d, too many scsi devices configured. Ensure total disk and ISO scsi assignments don't exceed 31 devices", scsiCount))
							break
						}
						if !slices.Contains(cloneSourceDisks, fmt.Sprintf("scsi%d", scsiCount)) &&
							reflect.ValueOf(&scsiDisks).Elem().
								FieldByName(fmt.Sprintf("Disk_%d", scsiCount)).
								IsNil() {
							reflect.
								ValueOf(&scsiDisks).Elem().
								FieldByName(fmt.Sprintf("Disk_%d", scsiCount)).
								Set(reflect.ValueOf(&dev))
							isos[idx].AssignedDeviceIndex = fmt.Sprintf("scsi%d", scsiCount)
							scsiCount++
							break
						}
						log.Printf("scsi%d occupied, trying next device index", scsiCount)
						scsiCount++
					}
				}
			}
		}
	}

	return errs, warnings, &qemuStorages
}

func generateProxmoxPCIDeviceMap(devices []pciDeviceConfig) proxmox.QemuPciDevices {
	out := make(proxmox.QemuPciDevices, len(devices))
	for idx, d := range devices {
		pcie := d.PCIe
		primary := d.XVGA
		rombar := !d.HideROMBAR
		var mdev *proxmox.PciMediatedDevice
		if d.MDEV != "" {
			m := proxmox.PciMediatedDevice(d.MDEV)
			mdev = &m
		}
		var deviceID *proxmox.PciDeviceID
		if d.DeviceID != "" {
			id := proxmox.PciDeviceID(d.DeviceID)
			deviceID = &id
		}
		var subDeviceID *proxmox.PciSubDeviceID
		if d.SubDeviceID != "" {
			id := proxmox.PciSubDeviceID(d.SubDeviceID)
			subDeviceID = &id
		}
		var subVendorID *proxmox.PciSubVendorID
		if d.SubVendorID != "" {
			id := proxmox.PciSubVendorID(d.SubVendorID)
			subVendorID = &id
		}
		var vendorID *proxmox.PciVendorID
		if d.VendorID != "" {
			id := proxmox.PciVendorID(d.VendorID)
			vendorID = &id
		}
		var entry proxmox.QemuPci
		if d.Mapping != "" {
			mappingID := proxmox.ResourceMappingPciID(d.Mapping)
			entry.Mapping = &proxmox.QemuPciMapping{
				ID:          &mappingID,
				PCIe:        &pcie,
				PrimaryGPU:  &primary,
				ROMbar:      &rombar,
				MDev:        mdev,
				DeviceID:    deviceID,
				SubDeviceID: subDeviceID,
				SubVendorID: subVendorID,
				VendorID:    vendorID,
			}
		} else {
			raw := &proxmox.QemuPciRaw{
				PCIe:        &pcie,
				PrimaryGPU:  &primary,
				ROMbar:      &rombar,
				MDev:        mdev,
				DeviceID:    deviceID,
				SubDeviceID: subDeviceID,
				SubVendorID: subVendorID,
				VendorID:    vendorID,
			}
			if d.Host != "" {
				rawID := proxmox.PciID(d.Host)
				raw.ID = &rawID
			}
			entry.Raw = raw
		}
		out[proxmox.QemuPciID(idx)] = entry
	}
	return out
}

func generateProxmoxSerials(serials []string) proxmox.SerialInterfaces {
	devs := make(proxmox.SerialInterfaces)
	for idx, serial := range serials {
		switch serial {
		case "socket":
			devs[proxmox.SerialID(idx)] = proxmox.SerialInterface{
				Socket: true,
			}
		default:
			devs[proxmox.SerialID(idx)] = proxmox.SerialInterface{
				Path: proxmox.SerialPath(serial),
			}
		}
	}
	return devs
}

func generateProxmoxRng0(rng0 rng0Config) *proxmox.VirtIoRNG {
	if rng0 == (rng0Config{}) {
		return nil
	}
	out := &proxmox.VirtIoRNG{}
	if rng0.Source != "" {
		var source proxmox.EntropySource
		if err := source.Parse(rng0.Source); err == nil {
			out.Source = &source
		}
	}
	if rng0.MaxBytes > 0 {
		limit := uint(rng0.MaxBytes)
		out.Limit = &limit
	}
	if rng0.Period > 0 {
		period := time.Duration(rng0.Period) * time.Millisecond
		out.Period = &period
	}
	return out
}

func generateProxmoxVga(vga vgaConfig) proxmox.QemuDevice {
	dev := make(proxmox.QemuDevice)
	setDeviceParamIfDefined(dev, "type", vga.Type)

	if vga.Memory > 0 {
		dev["memory"] = vga.Memory
	}
	return dev
}

// arch is a create-only Proxmox setting; a post-create PUT silently fails and leaves the VM unbootable.
func generateProxmoxCreateOptions(arch string) *proxmox.QemuCreateOptions {
	if arch == "" {
		return nil
	}
	cpuArch := proxmox.CpuArchitecture(arch)
	return &proxmox.QemuCreateOptions{Architecture: &cpuArch}
}

func generateProxmoxEfi(efi efiConfig) *proxmox.EfiDisk {
	if efi == (efiConfig{}) {
		return nil
	}
	out := &proxmox.EfiDisk{}
	if efi.EFIStoragePool != "" {
		storage := proxmox.StorageName(efi.EFIStoragePool)
		out.Storage = &storage
	}
	if efi.EFIType != "" {
		efiType := proxmox.EfiDiskType(efi.EFIType)
		out.Type = &efiType
	}
	if efi.EFIFormat != "" {
		format := proxmox.QemuDiskFormat(efi.EFIFormat)
		out.Format = &format
	}
	preEnrolled := efi.PreEnrolledKeys
	out.PreEnrolledKeys = &preEnrolled
	return out
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
	StopVm(context.Context, *proxmox.VmRef) (string, error)
	DeleteVm(context.Context, *proxmox.VmRef) (string, error)
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
	_, err := client.StopVm(context.Background(), vmRef)
	if err != nil {
		ui.Error(fmt.Sprintf("Error stopping VM. Please stop and delete it manually: %s", err))
		return
	}

	ui.Say("Deleting VM")
	_, err = client.DeleteVm(context.Background(), vmRef)
	if err != nil {
		ui.Error(fmt.Sprintf("Error deleting VM. Please delete it manually: %s", err))
		return
	}
}
