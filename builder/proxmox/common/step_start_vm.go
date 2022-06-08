package proxmox

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

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
	GetNextID(int) (int, error)
	StartVm(*proxmox.VmRef) (string, error)
	SetVmConfig(*proxmox.VmRef, map[string]interface{}) (interface{}, error)
	GetVmRefsByName(vmName string) (vmrs []*proxmox.VmRef, err error)
	GetVmState(vmr *proxmox.VmRef) (vmState map[string]interface{}, err error)
	StopVm(vmr *proxmox.VmRef) (exitStatus string, err error)
	DeleteVm(vmr *proxmox.VmRef) (exitStatus string, err error)
}

var (
	maxDuplicateIDRetries = 3
)

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

	ui.Say("Creating VM")
	config := proxmox.ConfigQemu{
		Name:         c.VMName,
		Agent:        agent,
		QemuKVM:      &kvm,
		Boot:         c.Boot, // Boot priority, example: "order=virtio0;ide2;net0", virtio0:Disk0 -> ide0:CDROM -> net0:Network
		QemuCpu:      c.CPUType,
		Description:  "Packer ephemeral build VM",
		Memory:       c.Memory,
		QemuCores:    c.Cores,
		QemuSockets:  c.Sockets,
		QemuOs:       c.OS,
		Bios:         c.BIOS,
		EFIDisk:      generateProxmoxEfi(c.EFIConfig),
		Machine:      c.Machine,
		QemuVga:      generateProxmoxVga(c.VGA),
		QemuNetworks: generateProxmoxNetworkAdapters(c.NICs),
		QemuDisks:    generateProxmoxDisks(c.Disks),
		QemuSerials:  generateProxmoxSerials(c.Serials),
		Scsihw:       c.SCSIController,
		Onboot:       &c.Onboot,
	}

	if c.ReplaceExisting {
		ui.Say("Replace existing set, checking for existing VM or template by name")
		vmrs, err := client.GetVmRefsByName(c.TemplateName)
		if err != nil {
			// This can happen if no VMs found, so don't consider it to be an error
			ui.Say(fmt.Sprintf("Couldn't find existing VMs, continuing: %s", err.Error()))
		} else if len(vmrs) > 0 {
			for _, vmr := range vmrs {
				ui.Say(fmt.Sprintf("Stopping VM ID: %d", vmr.VmId()))
				_, err := client.StopVm(vmr)
				if err != nil {
					state.Put("error", err)
					ui.Error(fmt.Sprintf("Error stopping VM ID: %d, continuing: %s", vmr.VmId(), err.Error()))
					continue
				}
				// Wait until vm is stopped. Otherwise, deletion will fail.
				ui.Say("Waiting for VM to stop for up to 300 seconds")
				waited := 0
				for waited < 300 {
					vmState, err := client.GetVmState(vmr)
					if err == nil && vmState["status"] == "stopped" {
						break
					} else if err != nil {
						state.Put("error", err)
						ui.Error(fmt.Sprintf("Error getting VM state for VM ID: %d, continuing: %s", vmr.VmId(), err.Error()))
						continue
					}
					time.Sleep(1 * time.Second)
				}

				_, err = client.DeleteVm(vmr)
				if err != nil {
					state.Put("error", err)
					ui.Error(fmt.Sprintf("Error deleting VM: %s", err.Error()))
				} else {
					ui.Say("Successfully deleted VM")
				}
			}
		} else {
			ui.Say("No existing VMs found to delete")
		}
	}

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

	// proxmox-api-go assumes all QemuDisks are actually hard disks, not cd
	// drives, so we need to add them via a config update
	if len(c.AdditionalISOFiles) > 0 {
		addISOConfig := make(map[string]interface{})
		for _, iso := range c.AdditionalISOFiles {
			addISOConfig[iso.Device] = fmt.Sprintf("%s,media=cdrom", iso.ISOFile)
		}
		_, err := client.SetVmConfig(vmRef, addISOConfig)
		if err != nil {
			err := fmt.Errorf("Error updating template: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	// The EFI disk doesn't get created reliably when using the clone builder,
	// so let's make sure it's there.
	if c.EFIConfig != (efiConfig{}) && c.Ctx.BuildType == "proxmox-clone" {
		addEFIConfig := make(map[string]interface{})
		err := config.CreateQemuEfiParams(addEFIConfig)
		if err != nil {
			err := fmt.Errorf("error creating EFI parameters: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
		}
		_, err = client.SetVmConfig(vmRef, addEFIConfig)
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
		setDeviceParamIfDefined(devs[idx], "firewall", strconv.FormatBool(nics[idx].Firewall))

		if nics[idx].PacketQueues > 0 {
			devs[idx]["queues"] = nics[idx].PacketQueues
		}
	}
	return devs
}
func generateProxmoxDisks(disks []diskConfig) proxmox.QemuDevices {
	devs := make(proxmox.QemuDevices)
	for idx := range disks {
		devs[idx] = make(proxmox.QemuDevice)
		setDeviceParamIfDefined(devs[idx], "type", disks[idx].Type)
		setDeviceParamIfDefined(devs[idx], "size", disks[idx].Size)
		setDeviceParamIfDefined(devs[idx], "storage", disks[idx].StoragePool)
		setDeviceParamIfDefined(devs[idx], "storage_type", disks[idx].StoragePoolType)
		setDeviceParamIfDefined(devs[idx], "cache", disks[idx].CacheMode)
		setDeviceParamIfDefined(devs[idx], "format", disks[idx].DiskFormat)

		if devs[idx]["type"] == "scsi" || devs[idx]["type"] == "virtio" {
			setDeviceParamIfDefined(devs[idx], "iothread", strconv.FormatBool(disks[idx].IOThread))
		}
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

func setDeviceParamIfDefined(dev proxmox.QemuDevice, key, value string) {
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
