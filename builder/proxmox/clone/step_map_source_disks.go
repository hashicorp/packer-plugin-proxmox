// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmoxclone

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	proxmoxapi "github.com/Telmate/proxmox-api-go/proxmox"
	proxmox "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// StepMapSourceDisks retrieves the configuration of the clone source vm
// and identifies any attached disks to prevent hcl/json defined disks
// and isos from overwriting their assignments.
// (Enables append behavior for hcl/json defined disks and ISOs)
type StepMapSourceDisks struct{}

type cloneSource interface {
	GetVmConfig(*proxmoxapi.VmRef) (map[string]interface{}, error)
	GetVmRefsByName(string) ([]*proxmoxapi.VmRef, error)
	CheckVmRef(*proxmoxapi.VmRef) error
}

var _ cloneSource = &proxmoxapi.Client{}

func (s *StepMapSourceDisks) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(cloneSource)
	c := state.Get("clone-config").(*Config)

	var sourceVmr *proxmoxapi.VmRef
	if c.CloneVM != "" {
		sourceVmrs, err := client.GetVmRefsByName(c.CloneVM)
		if err != nil {
			state.Put("error", fmt.Errorf("Could not retrieve VM: %s", err))
			return multistep.ActionHalt
		}
		// prefer source Vm located on same node
		sourceVmr = sourceVmrs[0]
		for _, candVmr := range sourceVmrs {
			if candVmr.Node() == c.Node {
				sourceVmr = candVmr
			}
		}
	} else if c.CloneVMID != 0 {
		sourceVmr = proxmoxapi.NewVmRef(c.CloneVMID)
		err := client.CheckVmRef(sourceVmr)
		if err != nil {
			state.Put("error", fmt.Errorf("Could not retrieve VM: %s", err))
			return multistep.ActionHalt
		}
	}

	vmParams, err := client.GetVmConfig(sourceVmr)
	if err != nil {
		err := fmt.Errorf("error fetching template config: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	var sourceDisks []string

	// example v data returned for a disk:
	// local-lvm:base-9100-disk-1,backup=0,cache=none,discard=on,replicate=0,size=16G
	// example v data returned for a cloud-init disk:
	// local-lvm:vm-9100-cloudinit,media=cdrom
	// example v data returned for a cdrom:
	// local-lvm:iso/ubuntu-14.04.1-server-amd64.iso,media=cdrom,size=572M

	// preserve only disk assignments, cloud-init drives are recreated by common builder
	for k, v := range vmParams {
		// get device from k eg. ide from ide2
		rd := regexp.MustCompile(`\D+`)
		switch rd.FindString(k) {
		case "ide", "sata", "scsi", "virtio":
			if !strings.Contains(v.(string), "media=cdrom") {
				log.Println("disk discovered on source vm at", k)
				sourceDisks = append(sourceDisks, k)
			}
		}
	}

	// store discovered disks in common config
	d := state.Get("config").(*proxmox.Config)
	d.CloneSourceDisks = sourceDisks
	state.Put("config", d)

	return multistep.ActionContinue
}

func (s *StepMapSourceDisks) Cleanup(state multistep.StateBag) {}
