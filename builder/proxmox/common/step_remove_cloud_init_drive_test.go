// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"testing"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type CloudInitDriveRemoverMock struct {
	getConfig func() (map[string]interface{}, error)
	setConfig func(map[string]interface{}) (string, error)
}

func (m CloudInitDriveRemoverMock) GetVmConfig(*proxmox.VmRef) (map[string]interface{}, error) {
	return m.getConfig()
}
func (m CloudInitDriveRemoverMock) SetVmConfig(vmref *proxmox.VmRef, c map[string]interface{}) (interface{}, error) {
	return m.setConfig(c)
}

var _ CloudInitDriveRemover = CloudInitDriveRemoverMock{}

func TestRemoveCloudInitDrive(t *testing.T) {
	cs := []struct {
		name                string
		builderConfig       *Config
		initialVMConfig     map[string]interface{}
		getConfigErr        error
		expectCallSetConfig bool
		expectedDelete      string
		setConfigErr        error
		expectedAction      multistep.StepAction
	}{
		{
			name:          "remove Cloud-Init drive",
			builderConfig: &Config{},
			initialVMConfig: map[string]interface{}{
				"ide3": "local-zfs:vm-500-cloudinit,media=cdrom",
			},
			expectCallSetConfig: true,
			expectedDelete:      "ide3",
			expectedAction:      multistep.ActionContinue,
		},
		{
			name:          "leave other drives alone",
			builderConfig: &Config{},
			initialVMConfig: map[string]interface{}{
				"ide2":  "local:iso/debian-11.6.0-amd64-netinst.iso,media=cdrom,size=388M",
				"scsi0": "local-zfs:vm-108-disk-0,iothread=1,size=10G",
			},
			expectCallSetConfig: false,
			expectedDelete:      "",
			expectedAction:      multistep.ActionContinue,
		},
		{
			name:          "find Cloud-Init drive on all controllers",
			builderConfig: &Config{},
			initialVMConfig: map[string]interface{}{
				"ide3":   "local-zfs:vm-104-cloudinit,media=cdrom,size=4M",
				"sata5":  "local-zfs:vm-104-cloudinit,media=cdrom,size=4M",
				"scsi30": "local-zfs:vm-104-cloudinit,media=cdrom,size=4M",
			},
			expectCallSetConfig: true,
			expectedDelete:      "ide3,sata5,scsi30",
			expectedAction:      multistep.ActionContinue,
		},
		{
			name:          "remove cloud-init config options",
			builderConfig: &Config{},
			initialVMConfig: map[string]interface{}{
				"ciuser":       "root",
				"cipassword":   "$5$FUaXk2yX$2/E0AXhEfGJqxNA6H9ORURyS8WSnsm8uT9S4vvDqwd7",
				"sshkeys":      "ssh-ed25519%20AAAAC3NzaC1lZDI1NTE5AAAAIEGL6AvL7oe7nQThd8hr6%2FqKeYtQv3oG%2Fur1x2U3AovJ%20packer",
				"searchdomain": "example.com",
				"nameserver":   "9.9.9.9 149.112.112.112",
				"ipconfig0":    "ip=10.0.10.123/24,gw=10.0.10.1",
			},
			expectCallSetConfig: true,
			expectedDelete:      "cipassword,ciuser,nameserver,searchdomain,sshkeys,ipconfig0",
			expectedAction:      multistep.ActionContinue,
		},
	}

	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			remover := CloudInitDriveRemoverMock{
				getConfig: func() (map[string]interface{}, error) {
					return c.initialVMConfig, c.getConfigErr
				},
				setConfig: func(changes map[string]interface{}) (string, error) {
					if !c.expectCallSetConfig {
						t.Error("Did not expect SetVmConfig to be called")
					}
					if changes["delete"] != c.expectedDelete {
						t.Errorf("Expected delete to be %s, got %s", c.expectedDelete, changes["delete"])
					}

					return "", c.setConfigErr
				},
			}

			state := new(multistep.BasicStateBag)
			state.Put("ui", packersdk.TestUi(t))
			state.Put("config", c.builderConfig)
			state.Put("vmRef", proxmox.NewVmRef(1))
			state.Put("proxmoxClient", remover)

			step := stepRemoveCloudInitDrive{}
			action := step.Run(context.TODO(), state)
			if action != c.expectedAction {
				t.Errorf("Expected action to be %v, got %v", c.expectedAction, action)
			}
		})
	}
}
