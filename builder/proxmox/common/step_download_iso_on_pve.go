// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"encoding/hex"
	"fmt"
	"path"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/go-getter/v2"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// stepDownloadISOOnPVE downloads an ISO file directly to the specified PVE node.
// Checksums are also calculated and compared on the PVE node, not by Packer.
type stepDownloadISOOnPVE struct {
	ISO *additionalISOsConfig
}

func (s *stepDownloadISOOnPVE) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	var isoStoragePath string
	isoStoragePath = Download_iso_on_pve(state, s.ISO.ISOUrls, s.ISO.ISOChecksum, s.ISO.ISOStoragePool)

	if isoStoragePath != "" {
		s.ISO.ISOFile = isoStoragePath
		return multistep.ActionContinue
	}
	// Abort if no ISO can be downloaded
	return multistep.ActionHalt
}

func Download_iso_on_pve(state multistep.StateBag, ISOUrls []string, ISOChecksum string, ISOStoragePool string) string {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(*proxmox.Client)
	c := state.Get("config").(*Config)

	var isoConfig proxmox.ConfigContent_Iso
	for _, url := range ISOUrls {
		var checksum string
		var checksumType string
		if ISOChecksum == "none" {
			checksum = ""
			checksumType = ""
		} else {
			gr := &getter.Request{
				Src: url + "?checksum=" + ISOChecksum,
			}
			gc := getter.Client{}
			fileChecksum, err := gc.GetChecksum(context.TODO(), gr)
			if err != nil {
				state.Put("error", err)
				ui.Error(err.Error())
				continue
			}
			checksum = hex.EncodeToString(fileChecksum.Value)
			checksumType = fileChecksum.Type
		}

		isoConfig = proxmox.ConfigContent_Iso{
			Node:              c.Node,
			Storage:           ISOStoragePool,
			DownloadUrl:       url,
			Filename:          path.Base(url),
			ChecksumAlgorithm: checksumType,
			Checksum:          checksum,
		}

		ui.Say(fmt.Sprintf("Beginning download of %s to node %s", isoConfig.DownloadUrl, isoConfig.Node))
		err := proxmox.DownloadIsoFromUrl(client, isoConfig)
		if err != nil {
			state.Put("error", err)
			ui.Error(err.Error())
			continue
		}
		isoStoragePath := fmt.Sprintf("%s:iso/%s", isoConfig.Storage, isoConfig.Filename)
		ui.Say(fmt.Sprintf("Finished downloading %s", isoStoragePath))
		return isoStoragePath
	}
	return ""
}

func (s *stepDownloadISOOnPVE) Cleanup(state multistep.StateBag) {
}
