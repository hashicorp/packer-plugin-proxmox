// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmoxiso

import (
	"context"

	proxmoxapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/hcl/v2/hcldec"
	proxmox "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// The unique id for the builder
const BuilderID = "proxmox.iso"

type Builder struct {
	config Config
}

// Builder implements packersdk.Builder
var _ packersdk.Builder = &Builder{}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec { return b.config.FlatMapstructure().HCL2Spec() }

func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	return b.config.Prepare(raws...)
}

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	state := new(multistep.BasicStateBag)

	// prepend boot iso device to any defined additional_isos
	var isoArray []proxmox.ISOsConfig
	isoArray = append(isoArray, b.config.BootISO)
	isoArray = append(isoArray, b.config.ISOs...)
	b.config.ISOs = isoArray

	state.Put("iso-config", &b.config)

	preSteps := []multistep.Step{}
	postSteps := []multistep.Step{}

	sb := proxmox.NewSharedBuilder(BuilderID, b.config.Config, preSteps, postSteps, &isoVMCreator{})
	return sb.Run(ctx, ui, hook, state)
}

type isoVMCreator struct{}

func (*isoVMCreator) Create(vmRef *proxmoxapi.VmRef, config proxmoxapi.ConfigQemu, state multistep.StateBag) error {
	client := state.Get("proxmoxClient").(*proxmoxapi.Client)
	return config.Create(vmRef, client)
}
