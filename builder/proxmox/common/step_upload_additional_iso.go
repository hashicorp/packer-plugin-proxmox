// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	proxmoxapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// stepUploadAdditionalISO uploads an ISO file
type stepUploadAdditionalISO struct {
	ISO *additionalISOsConfig
}

type uploader interface {
	Upload(node string, storage string, contentType string, filename string, file io.Reader) error
	DeleteVolume(vmr *proxmoxapi.VmRef, storageName string, volumeName string) (exitStatus interface{}, err error)
}

var _ uploader = &proxmoxapi.Client{}

func (s *stepUploadAdditionalISO) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(uploader)
	c := state.Get("config").(*Config)

	if !s.ISO.ShouldUploadISO {
		return multistep.ActionContinue
	}

	if len(s.ISO.CDFiles) > 0 || len(s.ISO.CDContent) > 0 {
		// output from commonsteps.StepCreateCD should have populate cd_path
		if cdPath, ok := state.GetOk("cd_path"); ok {
			state.Put(s.ISO.DownloadPathKey, cdPath.(string))
			state.Remove("cd_path")
		} else {
			err := fmt.Errorf("expected cd_path from commonsteps.StepCreateCD to be set")
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}

	p := state.Get(s.ISO.DownloadPathKey).(string)
	if p == "" {
		err := fmt.Errorf("path to downloaded ISO was empty")
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	isoPath, err := filepath.EvalSymlinks(p)
	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	r, err := os.Open(isoPath)
	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	filename := filepath.Base(isoPath)
	err = client.Upload(c.Node, s.ISO.ISOStoragePool, "iso", filename, r)
	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	isoStoragePath := fmt.Sprintf("%s:iso/%s", s.ISO.ISOStoragePool, filename)
	s.ISO.ISOFile = isoStoragePath
	ui.Message(fmt.Sprintf("Uploaded ISO to %s", isoStoragePath))

	return multistep.ActionContinue
}

func (s *stepUploadAdditionalISO) Cleanup(state multistep.StateBag) {
	c := state.Get("config").(*Config)
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(uploader)

	if (len(s.ISO.CDFiles) > 0 || len(s.ISO.CDContent) > 0) && s.ISO.DownloadPathKey != "" {
		// Fake a VM reference, DeleteVolume just needs the node to be valid
		vmRef := &proxmoxapi.VmRef{}
		vmRef.SetNode(c.Node)
		vmRef.SetVmType("qemu")

		_, err := client.DeleteVolume(vmRef, s.ISO.ISOStoragePool, s.ISO.ISOFile)
		if err != nil {
			state.Put("error", err)
			ui.Error(fmt.Sprintf("delete volume failed: %s", err.Error()))
			return
		}
		ui.Message(fmt.Sprintf("Deleted generated ISO from %s", s.ISO.ISOFile))
	}
}
