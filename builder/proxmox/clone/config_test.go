// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmoxclone

import (
	"strings"
	"testing"

	proxmox "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func mandatoryConfig(t *testing.T) map[string]interface{} {
	return map[string]interface{}{
		"proxmox_url":  "https://my-proxmox.my-domain:8006/api2/json",
		"username":     "apiuser@pve",
		"token":        "xxxx-xxxx-xxxx-xxxx",
		"node":         "my-proxmox",
		"ssh_username": "root",
		"clone_vm":     "MyTemplate",
	}
}

func TestRequiredParameters(t *testing.T) {
	var c Config
	_, _, err := c.Prepare(&c, make(map[string]interface{}))
	if err == nil {
		t.Fatal("Expected empty configuration to fail")
	}
	errs, ok := err.(*packersdk.MultiError)
	if !ok {
		t.Fatal("Expected errors to be packersdk.MultiError")
	}

	required := []string{"username", "token", "proxmox_url", "node", "ssh_username", "clone_vm", "clone_vm_id"}
	for _, param := range required {
		found := false
		for _, err := range errs.Errors {
			if strings.Contains(err.Error(), param) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected error about missing parameters %q", param)
		}
	}
}

func TestVMNameOrID(t *testing.T) {
	ipconfigTest := []struct {
		name          string
		cloneVM       string
		cloneVMID     int
		expectFailure bool
	}{
		{
			name:          "clone_vm given, no error",
			cloneVM:       "myVM",
			cloneVMID:     0,
			expectFailure: false,
		},
		{
			name:          "valid clone_vm_id given, no error",
			cloneVM:       "",
			cloneVMID:     100,
			expectFailure: false,
		},
		{
			name:          "clone_vm_id out of range, error",
			cloneVM:       "",
			cloneVMID:     50,
			expectFailure: true,
		},
		{
			name:          "clone_vm and clone_vm_id given, error",
			cloneVM:       "myVM",
			cloneVMID:     100,
			expectFailure: true,
		},
		{
			name:          "neither clone_vm nor clone_vm_id given, error",
			cloneVM:       "",
			cloneVMID:     0,
			expectFailure: true,
		},
	}

	for _, tt := range ipconfigTest {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mandatoryConfig(t)
			cfg["clone_vm"] = tt.cloneVM
			cfg["clone_vm_id"] = tt.cloneVMID

			var c Config
			_, _, err := c.Prepare(&c, cfg)
			if err != nil && !tt.expectFailure {
				t.Fatalf("unexpected failure: %s", err)
			}
			if err == nil && tt.expectFailure {
				t.Errorf("expected failure, but prepare succeeded")
			}
		})
	}
}

func TestNameserver(t *testing.T) {
	ipconfigTest := []struct {
		name          string
		nameserver    string
		expectFailure bool
	}{
		{
			name:          "nameserver empty, no error",
			expectFailure: false,
			nameserver:    "",
		},
		{
			name:          "single valid nameserver, no error",
			expectFailure: false,
			nameserver:    "192.168.1.1",
		},
		{
			name:          "two valid nameservers, no error",
			expectFailure: false,
			nameserver:    "192.168.1.1 192.168.1.2",
		},
		{
			name:          "comma separated nameservers, fail",
			expectFailure: true,
			nameserver:    "192.168.1.1,192.168.1.2",
		},
		{
			name:          "invalid nameserver, fail",
			expectFailure: true,
			nameserver:    "192.168.1",
		},
	}

	for _, tt := range ipconfigTest {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mandatoryConfig(t)
			cfg["nameserver"] = tt.nameserver

			var c Config
			_, _, err := c.Prepare(&c, cfg)
			if err != nil && !tt.expectFailure {
				t.Fatalf("unexpected failure: %s", err)
			}
			if err == nil && tt.expectFailure {
				t.Errorf("expected failure, but prepare succeeded")
			}
		})
	}
}

func TestIpconfig(t *testing.T) {
	ipconfigTest := []struct {
		name          string
		nics          []proxmox.NICConfig
		ipconfigs     []cloudInitIpconfig
		expectFailure bool
	}{
		{
			name:          "ipconfig empty, no error",
			expectFailure: false,
			ipconfigs:     []cloudInitIpconfig{},
		},
		{
			name:          "valid ipconfig, no error",
			expectFailure: false,
			ipconfigs: []cloudInitIpconfig{
				{
					Ip:       "192.168.1.55/24",
					Gateway:  "192.168.1.1",
					Ip6:      "fda8:a260:6eda:20::4da/128",
					Gateway6: "fda8:a260:6eda:20::1",
				},
			},
			nics: []proxmox.NICConfig{
				{
					Model:  "virtio",
					Bridge: "vmbr0",
				},
			},
		},
		{
			name:          "IPv4 invalid CIDR, fail",
			expectFailure: true,
			ipconfigs: []cloudInitIpconfig{
				{
					Ip:      "192.168.1.55",
					Gateway: "192.168.1.1",
				},
			},
			nics: []proxmox.NICConfig{
				{
					Model:  "virtio",
					Bridge: "vmbr0",
				},
			},
		},
		{
			name:          "IPv6 invalid CIDR, fail",
			expectFailure: true,
			ipconfigs: []cloudInitIpconfig{
				{
					Ip6:      "fda8:a260:6eda:20::4da",
					Gateway6: "fda8:a260:6eda:20::1",
				},
			},
			nics: []proxmox.NICConfig{
				{
					Model:  "virtio",
					Bridge: "vmbr0",
				},
			},
		},
		{
			name:          "not enough nics, fail",
			expectFailure: true,
			ipconfigs: []cloudInitIpconfig{
				{
					Ip6:      "fda8:a260:6eda:20::4da/128",
					Gateway6: "fda8:a260:6eda:20::1",
				},
				{
					Ip6:      "fda8:a260:6eda:20::4db/128",
					Gateway6: "fda8:a260:6eda:20::1",
				},
			},
			nics: []proxmox.NICConfig{
				{
					Model:  "virtio",
					Bridge: "vmbr0",
				},
			},
		},
		{
			name:          "ipconfig DHCP, no error",
			expectFailure: false,
			ipconfigs: []cloudInitIpconfig{
				{
					Ip:  "dhcp",
					Ip6: "dhcp",
				},
			},
			nics: []proxmox.NICConfig{
				{
					Model:  "virtio",
					Bridge: "vmbr0",
				},
			},
		},
	}

	for _, tt := range ipconfigTest {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mandatoryConfig(t)
			cfg["network_adapters"] = tt.nics
			cfg["ipconfig"] = tt.ipconfigs

			var c Config
			_, _, err := c.Prepare(&c, cfg)
			if err != nil && !tt.expectFailure {
				t.Fatalf("unexpected failure: %s", err)
			}
			if err == nil && tt.expectFailure {
				t.Errorf("expected failure, but prepare succeeded")
			}
		})
	}
}
