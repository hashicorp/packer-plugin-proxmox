package proxmoxiso

import (
	"context"
	"fmt"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// stepDownloadISOOnPVE downloads an ISO file directly to the specified PVE node.
// Checksums are also calculated and compared on the PVE node, not by Packer.
type stepDownloadISOOnPVE struct{}

func (s *stepDownloadISOOnPVE) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(*proxmox.Client)

	builderConfig := state.Get("iso-config").(*Config)

	if !builderConfig.shouldUploadISO {
		state.Put("iso_file", builderConfig.ISOFile)
		return multistep.ActionContinue
	}

	isoConfigs, err := builderConfig.generateIsoConfigs()
	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
	}
	for _, isoConfig := range isoConfigs {
		ui.Say(fmt.Sprintf("Beginning download of %s to node %s", isoConfig.DownloadUrl, isoConfig.Node))
		err := proxmox.DownloadIsoFromUrl(client, isoConfig)
		if err != nil {
			state.Put("error", err)
			ui.Error(err.Error())
			continue
		}
		isoStoragePath := fmt.Sprintf("%s:iso/%s", isoConfig.Storage, isoConfig.Filename)
		state.Put("iso_file", isoStoragePath)
		ui.Say(fmt.Sprintf("Finished downloading %s", isoStoragePath))
		return multistep.ActionContinue
	}
	// Abort if no ISO can be downloaded
	return multistep.ActionHalt
}

func (s *stepDownloadISOOnPVE) Cleanup(state multistep.StateBag) {
}
