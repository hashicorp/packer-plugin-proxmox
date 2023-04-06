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
	isoStoragePath = proxmoxcommon.Download_iso_on_pve(state, s.ISOUrls, s.ISOChecksum, s.ISOStoragePool)

	if isoStoragePath != "" {
		state.Put("iso_file", isoStoragePath)
		return multistep.ActionContinue
	}
	// Abort if no ISO can be downloaded
	return multistep.ActionHalt
}

func (s *stepDownloadISOOnPVE) Cleanup(state multistep.StateBag) {
}
