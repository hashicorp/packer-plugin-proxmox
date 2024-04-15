// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmoxiso

import (
	"context"
	"strings"

	proxmoxapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/hcl/v2/hcldec"
	proxmox "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
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

const downloadPathKey = "downloaded_iso_path"

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	state := new(multistep.BasicStateBag)
	state.Put("iso-config", &b.config)

	preSteps := []multistep.Step{}
	if b.config.ISODownloadPVE {
		preSteps = append(preSteps,
			&stepDownloadISOOnPVE{
				ISOStoragePool: b.config.ISOStoragePool,
				ISOUrls:        b.config.ISOUrls,
				ISOChecksum:    b.config.ISOChecksum,
			},
		)
	} else {
		preSteps = append(preSteps,
			&commonsteps.StepDownload{
				Checksum:    b.config.ISOChecksum,
				Description: "ISO",
				Extension:   b.config.TargetExtension,
				ResultKey:   downloadPathKey,
				TargetPath:  b.config.TargetPath,
				Url:         b.config.ISOUrls,
			},
			&stepUploadISO{},
		)
	}

	postSteps := []multistep.Step{
		&stepFinalizeISOTemplate{},
	}

	sb := proxmox.NewSharedBuilder(BuilderID, b.config.Config, preSteps, postSteps, &isoVMCreator{})
	return sb.Run(ctx, ui, hook, state)
}

type isoVMCreator struct{}

func (*isoVMCreator) Create(vmRef *proxmoxapi.VmRef, config proxmoxapi.ConfigQemu, state multistep.StateBag) error {
	isoFile := strings.Split(state.Get("iso_file").(string), ":iso/")
	config.Iso = &proxmoxapi.IsoFile{
		File:    isoFile[1],
		Storage: isoFile[0],
	}

	client := state.Get("proxmoxClient").(*proxmoxapi.Client)
	return config.Create(vmRef, client)
}
