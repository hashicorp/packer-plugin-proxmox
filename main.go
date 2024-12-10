// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"os"

	"github.com/hashicorp/packer-plugin-sdk/plugin"

	proxmoxclone "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/clone"
	proxmoxiso "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/iso"
	"github.com/hashicorp/packer-plugin-proxmox/datasource/virtualmachine"
	"github.com/hashicorp/packer-plugin-proxmox/version"
)

func main() {
	pps := plugin.NewSet()
	// When the builder was split, the alias "proxmox" was added to Packer for the iso builder.
	// Registering 'plugin.DEFAULT_NAME' does the same for the external plugin.
	pps.RegisterBuilder(plugin.DEFAULT_NAME, new(proxmoxiso.Builder))
	pps.RegisterBuilder("iso", new(proxmoxiso.Builder))
	pps.RegisterBuilder("clone", new(proxmoxclone.Builder))
	pps.RegisterDatasource("virtualmachine", new(virtualmachine.Datasource))
	pps.SetVersion(version.PluginVersion)
	err := pps.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
