// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"encoding/hex"
	"errors"
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
	ui := state.Get("ui").(packersdk.Ui)

	var isoStoragePath string
	isoStoragePath, err := DownloadISOOnPVE(state, s.ISO.ISOUrls, s.ISO.ISOChecksum, s.ISO.ISOStoragePool)

	// Abort if no ISO can be downloaded
	if err != nil {
		state.Put("error", err)
		ui.Error("Download of iso file failed. Aborting!")
		return multistep.ActionHalt
	}
	// If available, set the file path to the downloaded iso file on the node
	s.ISO.ISOFile = isoStoragePath
	return multistep.ActionContinue
}

// DownloadISOOnPVE abstracts the checksum and download process os that the code can be shared between
// the common module and the iso module. This is necessary because both handle the storage path to the iso differently.
//
// The function takes a list of URLs to download the iso and tries them one another.
// If a download was successful it skips the additonal downlaod mirrors and returns the path to the iso on the node.
//
// Returns: When successful, the path to the iso on the node, else an empty string.
func DownloadISOOnPVE(state multistep.StateBag, ISOUrls []string, ISOChecksum string, ISOStoragePool string) (string, error) {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(*proxmox.Client)
	c := state.Get("config").(*Config)

	// Generate ISOConfig configuration attributes in the format defined for packer-plugin-sdk
	// and use go-getter to generate parameters compatible with the Proxmox-API.
	var isoConfig proxmox.ConfigContent_Iso
	for _, url := range ISOUrls {
		var checksum string
		var checksumType string
		if ISOChecksum != "none" {
			gr := &getter.Request{
				Src: url + "?checksum=" + ISOChecksum,
			}
			gc := getter.Client{}
			fileChecksum, err := gc.GetChecksum(context.TODO(), gr)
			if err != nil {
				state.Put("error", err)
				ui.Say(err.Error())
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
		// On error continues with the next URL and logs the error
		if err != nil {
			ui.Say(fmt.Sprintf("Download from %s failed!", isoConfig.DownloadUrl))
			continue
		}
		isoStoragePath := fmt.Sprintf("%s:iso/%s", isoConfig.Storage, isoConfig.Filename)
		ui.Say(fmt.Sprintf("Finished downloading %s", isoStoragePath))
		// Returns the path to the iso on the node
		return isoStoragePath, nil
	}
	// Returns an empty string, which means download was not successful.
	return "", errors.New("Couldn't download iso file from mirrors!")
}

func (s *stepDownloadISOOnPVE) Cleanup(state multistep.StateBag) {
}
