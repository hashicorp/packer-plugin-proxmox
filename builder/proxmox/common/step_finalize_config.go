// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// stepFinalizeConfig does any required modifications to the configuration _after_
// the VM has been converted into a template, such as updating name and description, or
// unmounting the installation ISO.
type stepFinalizeConfig struct{}

type finalizer interface {
	GetVmConfig(*proxmox.VmRef) (map[string]interface{}, error)
	SetVmConfig(*proxmox.VmRef, map[string]interface{}) (interface{}, error)
	StartVm(*proxmox.VmRef) (string, error)
	ShutdownVm(*proxmox.VmRef) (string, error)
}

var _ finalizer = &proxmox.Client{}

func (s *stepFinalizeConfig) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(finalizer)
	c := state.Get("config").(*Config)
	vmRef := state.Get("vmRef").(*proxmox.VmRef)

	changes := make(map[string]interface{})

	changes["name"] = c.VMName
	if c.TemplateName != "" {
		changes["name"] = c.TemplateName
	}

	// During build, the description is "Packer ephemeral build VM", so if no description is
	// set, we need to clear it
	changes["description"] = c.TemplateDescription

	vmParams, err := client.GetVmConfig(vmRef)
	if err != nil {
		err := fmt.Errorf("error fetching config: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	if c.CloudInit {
		cloudInitStoragePool := c.CloudInitStoragePool
		if cloudInitStoragePool == "" {
			if vmParams["bootdisk"] != nil && vmParams[vmParams["bootdisk"].(string)] != nil {
				bootDisk := vmParams[vmParams["bootdisk"].(string)].(string)
				cloudInitStoragePool = strings.Split(bootDisk, ":")[0]
			}
		}
		if cloudInitStoragePool != "" {
			var diskControllers []string
			switch c.CloudInitDiskType {
			// Proxmox supports up to 6 SATA controllers (0 - 5)
			case "sata":
				for i := 0; i < 6; i++ {
					sataController := fmt.Sprintf("sata%d", i)
					diskControllers = append(diskControllers, sataController)
				}
			// and up to 31 SCSI controllers (0 - 30)
			case "scsi":
				for i := 0; i < 31; i++ {
					scsiController := fmt.Sprintf("scsi%d", i)
					diskControllers = append(diskControllers, scsiController)
				}
			// Unspecified disk type defaults to "ide"
			case "ide":
				diskControllers = []string{"ide0", "ide1", "ide2", "ide3"}
			default:
				state.Put("error", fmt.Errorf("unsupported disk type %q", c.CloudInitDiskType))
				return multistep.ActionHalt
			}
			cloudInitAttached := false
			// find a free disk controller
			for _, controller := range diskControllers {
				if vmParams[controller] == nil {
					ui.Say("Adding a cloud-init cdrom in storage pool " + cloudInitStoragePool)
					changes[controller] = cloudInitStoragePool + ":cloudinit"
					cloudInitAttached = true
					break
				}
			}
			if cloudInitAttached == false {
				err := fmt.Errorf("Found no free controller of type %s for a cloud-init cdrom", c.CloudInitDiskType)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
		} else {
			err := fmt.Errorf("cloud_init is set to true, but cloud_init_storage_pool is empty and could not be set automatically. set cloud_init_storage_pool in your configuration")
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	deleteItems := []string{}
	if len(c.ISOs) > 0 {
		for idx := range c.ISOs {
			cdrom := c.ISOs[idx].AssignedDeviceIndex
			if c.ISOs[idx].Unmount {
				if vmParams[cdrom] == nil || !strings.Contains(vmParams[cdrom].(string), "media=cdrom") {
					err := fmt.Errorf("Cannot eject ISO from cdrom drive, %s is not present or not a cdrom media", cdrom)
					state.Put("error", err)
					ui.Error(err.Error())
					return multistep.ActionHalt
				}
				if c.ISOs[idx].KeepCDRomDevice {
					changes[cdrom] = "none,media=cdrom"
				} else {
					deleteItems = append(deleteItems, cdrom)
				}
			} else {
				changes[cdrom] = c.ISOs[idx].ISOFile + ",media=cdrom"
			}
		}
	}

	// Disks that get replaced by the builder end up as unused disks -
	// find and remove them.
	rxUnused := regexp.MustCompile(`^unused\d+`)
	for key := range vmParams {
		if unusedDisk := rxUnused.FindString(key); unusedDisk != "" {
			deleteItems = append(deleteItems, unusedDisk)
		}
	}

	changes["delete"] = strings.Join(deleteItems, ",")

	if len(changes) > 0 {
		// Adding a Cloud-Init drive or removing CD-ROM devices won't take effect without a power off and on of the QEMU VM
		if c.SkipConvertToTemplate {
			ui.Say("Hardware changes pending for VM, stopping VM")
			_, err := client.ShutdownVm(vmRef)
			if err != nil {
				err := fmt.Errorf("Error converting VM to template, could not stop: %s", err)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
		}
		_, err := client.SetVmConfig(vmRef, changes)
		if err != nil {
			err := fmt.Errorf("Error updating template: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	// When build artifact is to be a VM, return a running VM
	if c.SkipConvertToTemplate {
		ui.Say("Resuming VM")
		_, err := client.StartVm(vmRef)
		if err != nil {
			err := fmt.Errorf("Error starting VM: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	return multistep.ActionContinue
}

func (s *stepFinalizeConfig) Cleanup(state multistep.StateBag) {}
