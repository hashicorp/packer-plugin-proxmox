package proxmox

import (
	"context"
	"fmt"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// stepStopVM takes the running VM configured in earlier steps ands stops it.

type stepStopVM struct{}

type vmTerminator interface {
	ShutdownVm(*proxmox.VmRef) (string, error)
}

var _ templateConverter = &proxmox.Client{}

func (s *stepStopVM) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(vmTerminator)
	vmRef := state.Get("vmRef").(*proxmox.VmRef)

	ui.Say("Stopping VM")
	_, err := client.ShutdownVm(vmRef)
	if err != nil {
		err := fmt.Errorf("Error converting VM to template, could not stop: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func (s *stepStopVM) Cleanup(state multistep.StateBag) {}
