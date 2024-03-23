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

// stepFinalizeTemplateConfig does any required modifications to the configuration _after_
// the VM has been converted into a template, such as updating name and description, or
// unmounting the installation ISO.
type stepFinalizeTemplateConfig struct{}

type templateFinalizer interface {
	GetVmConfig(*proxmox.VmRef) (map[string]interface{}, error)
	SetVmConfig(*proxmox.VmRef, map[string]interface{}) (interface{}, error)
}

var _ templateFinalizer = &proxmox.Client{}

func (s *stepFinalizeTemplateConfig) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(templateFinalizer)
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
		err := fmt.Errorf("error fetching template config: %s", err)
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
			default:
				// Unspecified disk type defaults to "ide"
				diskControllers = []string{"ide0", "ide1", "ide2", "ide3"}
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

	if len(c.AdditionalISOFiles) > 0 {
		for idx := range c.AdditionalISOFiles {
			cdrom := c.AdditionalISOFiles[idx].Device
			if c.AdditionalISOFiles[idx].Unmount {
				if vmParams[cdrom] == nil || !strings.Contains(vmParams[cdrom].(string), "media=cdrom") {
					err := fmt.Errorf("Cannot eject ISO from cdrom drive, %s is not present or not a cdrom media", cdrom)
					state.Put("error", err)
					ui.Error(err.Error())
					return multistep.ActionHalt
				}
				changes[cdrom] = "none,media=cdrom"
			} else {
				changes[cdrom] = c.AdditionalISOFiles[idx].ISOFile + ",media=cdrom"
			}
		}
	}

	// Disks that get replaced by the builder end up as unused disks -
	// find and remove them.
	rxUnused := regexp.MustCompile(`^unused\d+`)
	unusedDisks := []string{}
	for key := range vmParams {
		if unusedDisk := rxUnused.FindString(key); unusedDisk != "" {
			unusedDisks = append(unusedDisks, unusedDisk)
		}
	}
	changes["delete"] = strings.Join(unusedDisks, ",")

	if len(changes) > 0 {
		_, err := client.SetVmConfig(vmRef, changes)
		if err != nil {
			err := fmt.Errorf("Error updating template: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	return multistep.ActionContinue
}

func (s *stepFinalizeTemplateConfig) Cleanup(state multistep.StateBag) {}
