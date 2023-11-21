// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmoxiso

import (
	"context"

	proxmoxcommon "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

// stepDownloadISOOnPVE downloads an ISO file directly to the specified PVE node.
// Checksums are also calculated and compared on the PVE node, not by Packer.
type stepDownloadISOOnPVE struct {
	ISOStoragePool string
	ISOUrls        []string
	ISOChecksum    string
}

func (s *stepDownloadISOOnPVE) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	builderConfig := state.Get("iso-config").(*Config)
	if !builderConfig.shouldUploadISO {
		state.Put("iso_file", builderConfig.ISOFile)
		return multistep.ActionContinue
	}

	var isoStoragePath string
	isoStoragePath, err := proxmoxcommon.DownloadISOOnPVE(state, s.ISOUrls, s.ISOChecksum, s.ISOStoragePool)

	// Abort if no ISO can be downloaded
	if err != nil {
		state.Put("error", err)
		return multistep.ActionHalt
	}
	// If available, set the file path to the downloaded iso file on the node
	state.Put("iso_file", isoStoragePath)
	return multistep.ActionContinue
}

func (s *stepDownloadISOOnPVE) Cleanup(state multistep.StateBag) {
}
