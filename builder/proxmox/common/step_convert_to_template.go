// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"fmt"
	"log"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// stepConvertToTemplate takes the running VM configured in earlier steps, stops it, and
// converts it into a Proxmox template.
//
// It sets the artifact_id state which is used for Artifact lookup.
type stepConvertToTemplate struct{}

type templateConverter interface {
	ShutdownVm(*proxmox.VmRef) (string, error)
	CreateTemplate(*proxmox.VmRef) error
}

var _ templateConverter = &proxmox.Client{}

func (s *stepConvertToTemplate) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(templateConverter)
	vmRef := state.Get("vmRef").(*proxmox.VmRef)
	c := state.Get("config").(*Config)

	if c.SkipConvertToTemplate {
		ui.Say("skip_convert_to_template set, skipping conversion to template")
		state.Put("artifact_type", "VM")
	} else {
		ui.Say("Stopping VM")
		_, err := client.ShutdownVm(vmRef)
		if err != nil {
			err := fmt.Errorf("Error converting VM to template, could not stop: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		ui.Say("Converting VM to template")
		err = client.CreateTemplate(vmRef)
		if err != nil {
			err := fmt.Errorf("Error converting VM to template: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		state.Put("artifact_type", "template")
	}

	log.Printf("artifact_id: %d", vmRef.VmId())
	state.Put("artifact_id", vmRef.VmId())

	return multistep.ActionContinue
}

func (s *stepConvertToTemplate) Cleanup(state multistep.StateBag) {}
