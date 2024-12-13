// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"os"
	"slices"
	"strings"
	"fmt"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	common "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

type StepUpdateCloudInitSSH struct{}

func (s *StepUpdateCloudInitSSH) Run(ctx context.Context, state multiste.StateBag) multistep.StepAction {
	// NOTE: Can pass Cloud-Init data via CD or HTTP Server

	ui := state.Get("ui").(packersdk.Ui)
	c := state.Get("config").(*common.Config)

	if c.Comm.SSHTemporaryKeyPairName == "" ||
		(len(c.ISOs) <= 1 &&
		 c.HTTPConfig.HTTPDir == "" &&
		 len(c.HTTPConfig.HTTPContent) == 0) {
		return multistep.ActionContinue
	}

	temp_ssh_public_key := string(c.Comm.SSHPublicKey)

	if len(c.ISOs) > 1 {
		// Skip first ISO as it should be the BootISO
		for idx := range c.ISOs[1:] {
			if c.ISOs[idx].CDConfig.CDLabel != "cidata" {
				continue
			} 
			
			if value, ok := c.ISOs[idx].CDConfig.CDContent["user-data"]; ok {
				c.ISOs[idx].CDConfig.CDContent["user-data"] = strings.ReplaceAll(value, "${temporary_ssh_public_key}", temp_ssh_public_key)
				ui.Say("Updated 'user-data' CD Content to use temporary SSH public key")
				// CDContent will take precedence over CDFiles
				break
			}

			// TODO: This needs to be able to handle when directories and globs 
			// are passed to CDFiles
			for jdx, value := range c.ISOs[idx].CDConfig.CDFiles {
				if slices.Contains(value, "user-data") {
					dat, err := os.ReadFile(c.ISOs[idx].CDConfig.CDFiles[jdx])
					if err != nil {
						// It is ok if file does not exist
						break
					}
					// Choosing to write CDContent to avoid overwriting original
					// file or creating a new file with the temporary public key
					c.ISOs[idx].CDConfig.CDContent["user-data"] = strings.ReplaceAll(dat, "${temporary_ssh_public_key}", temp_ssh_public_key)
					ui.Say("Created 'user-data' CD Content to override CD File and use temporary SSH public key")
					// There can only be one user-data file
					break
				}
			}
		}
	}
	
	if c.HTTPConfig.HTTPDir != "" {
		user_data_file := fmt.Sprintf("%s/user-data", c.HTTPConfig.HTTPDir)

		dat, err := os.ReadFile(user_data_file)
		if err != nil {
			// It is ok if file does not exist
			return multistep.ActionContinue	
		}

		// Can't just write to "HTTPContent".  Since it directly conflicts with
		// "HTTPDir", we would have to walk and load every file from "HTTPDir" 
		// into "HTTPContent"
		err := os.WriteFile(user_data_file, strings.ReplaceAll(dat, "${temporary_ssh_public_key}", temp_ssh_public_key), 0600)
		if err == nil {
			ui.Say(fmt.Sprintf("Rewrote '%s' in HTTPDir to use temporary SSH public key", user_data_file))
		} else {
			ui.Say(fmt.Sprintf("Failed to rewrite '%s' in HTTPDir to use temporary SSH public key", user_data_file))
		}
	} else if len(c.HTTPConfig.HTTPContent) > 0 {
		if value, ok := c.HTTPConfig.HTTPContent["user-data"]; ok {
			c.HTTPConfig.HTTPContent["user-data"] = strings.ReplaceAll(value, "%{temporary_ssh_public_key}", temp_ssh_public_key)
			ui.Say("Updated 'user-data' HTTP Content to use temporary SSH public key")
		}
	}

	return multistep.ActionContinue
}