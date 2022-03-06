package proxmox

import (
	"context"
	"fmt"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// stepConvertToTemplate takes the stopped VM configured in earlier steps,
// and converts it into a Proxmox VM template.
type stepConvertToTemplate struct{}

type templateConverter interface {
	CreateTemplate(*proxmox.VmRef) error
}

var _ templateConverter = &proxmox.Client{}

func (s *stepConvertToTemplate) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(templateConverter)
	vmRef := state.Get("vmRef").(*proxmox.VmRef)

	ui.Say("Converting VM to template")
	var err = client.CreateTemplate(vmRef)
	if err != nil {
		err := fmt.Errorf("Error converting VM to template: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func (s *stepConvertToTemplate) Cleanup(state multistep.StateBag) {}
