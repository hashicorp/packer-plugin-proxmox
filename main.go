package main

import (
	"fmt"
	"os"

	"github.com/hashicorp/packer-plugin-sdk/plugin"

	proxmoxclone "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/clone"
	proxmoxiso "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/iso"
	"github.com/hashicorp/packer-plugin-proxmox/version"
)

func main() {
	pps := plugin.NewSet()
	pps.RegisterBuilder(plugin.DEFAULT_NAME, new(proxmoxiso.Builder))
	pps.RegisterBuilder("proxmox-iso", new(proxmoxiso.Builder))
	pps.RegisterBuilder("proxmox-clone", new(proxmoxclone.Builder))
	pps.SetVersion(version.PluginVersion)
	err := pps.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
