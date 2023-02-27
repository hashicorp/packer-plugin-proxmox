// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"fmt"
	"strings"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// stepRemoveCloudInitDrive removes the cloud-init cdrom and deletes
// VM config parameters related to cloud-init.
type stepRemoveCloudInitDrive struct{}

type CloudInitDriveRemover interface {
	GetVmConfig(*proxmox.VmRef) (map[string]interface{}, error)
	SetVmConfig(*proxmox.VmRef, map[string]interface{}) (interface{}, error)
}

var _ CloudInitDriveRemover = &proxmox.Client{}

func (s *stepRemoveCloudInitDrive) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(CloudInitDriveRemover)
	vmRef := state.Get("vmRef").(*proxmox.VmRef)

	changes := make(map[string]interface{})
	delete := []string{}

	vmParams, err := client.GetVmConfig(vmRef)
	if err != nil {
		err := fmt.Errorf("error fetching template config: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	// The cloud-init drive is usually attached to ide0, but check the other controllers as well
	diskControllers := []string{"ide0", "ide1", "ide2", "ide3"}
	// Proxmox supports up to 6 SATA controllers (0 - 5)
	for i := 0; i < 6; i++ {
		sataController := fmt.Sprintf("sata%d", i)
		diskControllers = append(diskControllers, sataController)
	}
	// and up to 31 SCSI controllers (0 - 30)
	for i := 0; i < 31; i++ {
		scsiController := fmt.Sprintf("scsi%d", i)
		diskControllers = append(diskControllers, scsiController)
	}

	for _, controller := range diskControllers {
		if vmParams[controller] != nil && strings.Contains(vmParams[controller].(string), "-cloudinit,media=cdrom") {
			delete = append(delete, controller)
		}
	}

	CloudInitParameters := []string{
		"cipassword",
		"ciuser",
		"nameserver",
		"searchdomain",
		"sshkeys",
	}
	for i := 0; i < 16; i++ {
		ipconfig := fmt.Sprintf("ipconfig%d", i)
		CloudInitParameters = append(CloudInitParameters, ipconfig)
	}
	for _, parameter := range CloudInitParameters {
		if vmParams[parameter] != nil {
			delete = append(delete, parameter)
		}
	}

	if len(delete) > 0 {
		changes["delete"] = strings.Join(delete, ",")

		_, err := client.SetVmConfig(vmRef, changes)
		if err != nil {
			err := fmt.Errorf("error updating template: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	return multistep.ActionContinue
}

func (s *stepRemoveCloudInitDrive) Cleanup(state multistep.StateBag) {
}
