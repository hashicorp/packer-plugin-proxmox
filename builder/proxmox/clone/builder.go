// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmoxclone

import (
	"crypto"
	"net/netip"
	"strings"

	proxmoxapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/hcl/v2/hcldec"
	proxmox "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"

	"context"
	"fmt"
)

// The unique id for the builder
const BuilderID = "proxmox.clone"

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
	state.Put("clone-config", &b.config)

	preSteps := []multistep.Step{
		&StepSshKeyPair{
			Debug:        b.config.PackerDebug,
			DebugKeyPath: fmt.Sprintf("%s.pem", b.config.PackerBuildName),
		},
		&StepMapSourceDisks{},
	}
	postSteps := []multistep.Step{}

	sb := proxmox.NewSharedBuilder(BuilderID, b.config.Config, preSteps, postSteps, &cloneVMCreator{})
	return sb.Run(ctx, ui, hook, state)
}

type cloneVMCreator struct{}

func (*cloneVMCreator) Create(vmRef *proxmoxapi.VmRef, config proxmoxapi.ConfigQemu, state multistep.StateBag) error {
	client := state.Get("proxmoxClient").(*proxmoxapi.Client)
	c := state.Get("clone-config").(*Config)
	comm := state.Get("config").(*proxmox.Config).Comm

	fullClone := 1
	if c.FullClone.False() {
		fullClone = 0
	}
	config.FullClone = &fullClone

	// cloud-init options

	var nameServers []netip.Addr
	if c.Nameserver != "" {
		for _, nameserver := range strings.Split(c.Nameserver, " ") {
			ip, _ := netip.ParseAddr(nameserver)
			nameServers = append(nameServers, ip)
		}
	}

	IpconfigMap := proxmoxapi.CloudInitNetworkInterfaces{}
	for idx := range c.Ipconfigs {
		if c.Ipconfigs[idx] != (cloudInitIpconfig{}) {

			// backwards compatibility conversions

			var ipv4cfg proxmoxapi.CloudInitIPv4Config
			var ipv6cfg proxmoxapi.CloudInitIPv6Config

			// cloudInitIpconfig.Ip accepts a CIDR address or 'dhcp' string
			switch c.Ipconfigs[idx].Ip {
			case "dhcp":
				ipv4cfg.DHCP = true
			default:
				if c.Ipconfigs[idx].Ip != "" {
					addr := proxmoxapi.IPv4CIDR(c.Ipconfigs[idx].Ip)
					ipv4cfg.Address = &addr
				}
			}
			if c.Ipconfigs[idx].Gateway != "" {
				gw := proxmoxapi.IPv4Address(c.Ipconfigs[idx].Gateway)
				ipv4cfg.Gateway = &gw
			}

			// cloudInitIpconfig.Ip6 accepts a CIDR address, 'auto' or 'dhcp' string
			switch c.Ipconfigs[idx].Ip6 {
			case "dhcp":
				ipv6cfg.DHCP = true
			case "auto":
				ipv6cfg.SLAAC = true
			default:
				if c.Ipconfigs[idx].Ip6 != "" {
					addr := proxmoxapi.IPv6CIDR(c.Ipconfigs[idx].Ip6)
					ipv6cfg.Address = &addr
				}
			}
			if c.Ipconfigs[idx].Gateway6 != "" {
				addr := proxmoxapi.IPv6Address(c.Ipconfigs[idx].Gateway6)
				ipv6cfg.Gateway = &addr
			}

			IpconfigMap[proxmoxapi.QemuNetworkInterfaceID(idx)] = proxmoxapi.CloudInitNetworkConfig{
				IPv4: &ipv4cfg,
				IPv6: &ipv6cfg,
			}
		}
	}

	var publicKey []crypto.PublicKey

	if comm.SSHPublicKey != nil {
		publicKey = append(publicKey, crypto.PublicKey(string(comm.SSHPublicKey)))
	}

	config.CloudInit = &proxmoxapi.CloudInit{
		Username:      &comm.SSHUsername,
		PublicSSHkeys: &publicKey,
		DNS: &proxmoxapi.GuestDNS{
			NameServers:  &nameServers,
			SearchDomain: &c.Searchdomain,
		},
		NetworkInterfaces: IpconfigMap,
	}

	var sourceVmr *proxmoxapi.VmRef
	if c.CloneVM != "" {
		sourceVmrs, err := client.GetVmRefsByName(c.CloneVM)
		if err != nil {
			return err
		}

		// prefer source Vm located on same node
		sourceVmr = sourceVmrs[0]
		for _, candVmr := range sourceVmrs {
			if candVmr.Node() == vmRef.Node() {
				sourceVmr = candVmr
			}
		}
	} else if c.CloneVMID != 0 {
		sourceVmr = proxmoxapi.NewVmRef(c.CloneVMID)
		err := client.CheckVmRef(sourceVmr)
		if err != nil {
			return err
		}
	}

	err := config.CloneVm(sourceVmr, vmRef, client)
	if err != nil {
		return err
	}
	_, err = config.Update(false, vmRef, client)
	if err != nil {
		return err
	}
	return nil
}
