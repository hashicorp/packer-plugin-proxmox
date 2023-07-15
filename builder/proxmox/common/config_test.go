// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"fmt"
	"strings"
	"testing"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func mandatoryConfig(t *testing.T) map[string]interface{} {
	return map[string]interface{}{
		"proxmox_url":  "https://my-proxmox.my-domain:8006/api2/json",
		"username":     "apiuser@pve",
		"password":     "supersecret",
		"node":         "my-proxmox",
		"ssh_username": "root",
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

	required := []string{"username", "password", "proxmox_url", "node", "ssh_username"}
	for _, param := range required {
		found := false
		for _, err := range errs.Errors {
			if strings.Contains(err.Error(), param) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected error about missing parameter %q", required)
		}
	}
}

func TestAgentSetToFalse(t *testing.T) {
	cfg := mandatoryConfig(t)
	cfg["qemu_agent"] = false

	var c Config
	_, _, err := c.Prepare(&c, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if c.Agent.False() != true {
		t.Errorf("Expected Agent to be false, got %t", c.Agent.True())
	}
}

func TestPacketQueueSupportForNetworkAdapters(t *testing.T) {
	drivertests := []struct {
		expectedToFail bool
		model          string
	}{
		{expectedToFail: false, model: "virtio"},
		{expectedToFail: true, model: "e1000"},
		{expectedToFail: true, model: "e1000-82540em"},
		{expectedToFail: true, model: "e1000-82544gc"},
		{expectedToFail: true, model: "e1000-82545em"},
		{expectedToFail: true, model: "i82551"},
		{expectedToFail: true, model: "i82557b"},
		{expectedToFail: true, model: "i82559er"},
		{expectedToFail: true, model: "ne2k_isa"},
		{expectedToFail: true, model: "ne2k_pci"},
		{expectedToFail: true, model: "pcnet"},
		{expectedToFail: true, model: "rtl8139"},
		{expectedToFail: true, model: "vmxnet3"},
	}

	for _, tt := range drivertests {
		device := make(map[string]interface{})
		device["bridge"] = "vmbr0"
		device["model"] = tt.model
		device["packet_queues"] = 2

		devices := make([]map[string]interface{}, 0)
		devices = append(devices, device)

		cfg := mandatoryConfig(t)
		cfg["network_adapters"] = devices

		var c Config
		_, _, err := c.Prepare(&c, cfg)

		if tt.expectedToFail == true && err == nil {
			t.Error("expected config preparation to fail, but no error occured")
		}

		if tt.expectedToFail == false && err != nil {
			t.Errorf("expected config preparation to succeed, but %s", err.Error())
		}
	}
}

func TestVMandTemplateName(t *testing.T) {
	dnsnametests := []struct {
		expectedToFail bool
		name           string
	}{
		{expectedToFail: false, name: "packer"},
		{expectedToFail: false, name: "pac.ker"},
		{expectedToFail: true, name: "pac_ker"},
		{expectedToFail: true, name: "pac ker"},
	}

	for _, tt := range dnsnametests {

		cfg := mandatoryConfig(t)
		cfg["vm_name"] = tt.name
		cfg["template_name"] = tt.name

		var c Config
		_, _, err := c.Prepare(&c, cfg)

		if tt.expectedToFail == true && err == nil {
			t.Error("expected config preparation to fail, but no error occured")
		}

		if tt.expectedToFail == false && err != nil {
			t.Errorf("expected config preparation to succeed, but %s", err.Error())
		}
	}
}

func TestAdditionalISOs(t *testing.T) {
	additionalisotests := []struct {
		name               string
		expectedToFail     bool
		additionalISOFiles map[string]interface{}
	}{
		{
			name:           "missing ISO definition should error",
			expectedToFail: true,
			additionalISOFiles: map[string]interface{}{
				"device": "ide1",
			},
		},
		{
			name:           "cd_files and iso_file specified should fail",
			expectedToFail: true,
			additionalISOFiles: map[string]interface{}{
				"device": "ide1",
				"cd_files": []string{
					"config_test.go",
				},
				"iso_file": "local:iso/test.iso",
			},
		},
		{
			name:           "cd_files, iso_file and iso_url specified should fail",
			expectedToFail: true,
			additionalISOFiles: map[string]interface{}{
				"device": "ide1",
				"cd_files": []string{
					"config_test.go",
				},
				"iso_file":         "local:iso/test.iso",
				"iso_url":          "http://example.com",
				"iso_checksum":     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"iso_storage_pool": "local",
			},
		},
		{
			name:           "missing iso_storage_pool should error",
			expectedToFail: true,
			additionalISOFiles: map[string]interface{}{
				"device": "ide1",
				"cd_files": []string{
					"config_test.go",
				},
			},
		},
		{
			name:           "cd_files valid should succeed",
			expectedToFail: false,
			additionalISOFiles: map[string]interface{}{
				"device": "ide1",
				"cd_files": []string{
					"config_test.go",
				},
				"iso_storage_pool": "local",
			},
		},
		{
			name:           "cd_content valid should succeed",
			expectedToFail: false,
			additionalISOFiles: map[string]interface{}{
				"device": "ide1",
				"cd_content": map[string]string{
					"test": "config_test.go",
				},
				"iso_storage_pool": "local",
			},
		},
		{
			name:           "iso_url valid should succeed",
			expectedToFail: false,
			additionalISOFiles: map[string]interface{}{
				"device":           "ide1",
				"iso_url":          "http://example.com",
				"iso_checksum":     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"iso_storage_pool": "local",
			},
		},
		{
			name:           "iso_file valid should succeed",
			expectedToFail: false,
			additionalISOFiles: map[string]interface{}{
				"device":   "ide1",
				"iso_file": "local:iso/test.iso",
			},
		},
	}

	for _, c := range additionalisotests {
		t.Run(c.name, func(t *testing.T) {
			cfg := mandatoryConfig(t)
			cfg["additional_iso_files"] = c.additionalISOFiles

			var config Config
			_, _, err := config.Prepare(&config, cfg)

			if c.expectedToFail == true && err == nil {
				t.Error("expected config preparation to fail, but no error occured")
			}

			if c.expectedToFail == false && err != nil {
				t.Errorf("expected config preparation to succeed, but %s", err.Error())
			}
		})
	}

}

func TestRng0(t *testing.T) {
	Rng0Test := []struct {
		name          string
		rng_config    rng0Config
		expectFailure bool
	}{
		{
			name:          "no error",
			expectFailure: false,
			rng_config: rng0Config{
				Source:   "/dev/urandom",
				MaxBytes: 1024,
				Period:   1000,
			},
		},
		{
			name:          "empty Source, error",
			expectFailure: true,
			rng_config: rng0Config{
				Source:   "",
				MaxBytes: 1024,
				Period:   1000,
			},
		},
		{
			name:          "negative Period, error",
			expectFailure: true,
			rng_config: rng0Config{
				Source:   "/dev/urandom",
				MaxBytes: 1024,
				Period:   -10,
			},
		},
		{
			name:          "zero Period, noerror",
			expectFailure: false,
			rng_config: rng0Config{
				Source:   "/dev/urandom",
				MaxBytes: 1024,
				Period:   0,
			},
		},
		{
			name:          "malformed Source error, error",
			expectFailure: true,
			rng_config: rng0Config{
				Source:   "/dev/abcde",
				MaxBytes: 1024,
				Period:   1000,
			},
		},
		{
			name:          "negative Period, error",
			expectFailure: true,
			rng_config: rng0Config{
				Source:   "/dev/urandom",
				MaxBytes: 1024,
				Period:   -10,
			},
		},
	}

	for _, tt := range Rng0Test {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mandatoryConfig(t)
			cfg["rng0"] = &tt.rng_config

			var c Config
			_, _, err := c.Prepare(&c, cfg)
			if err != nil {
				if !tt.expectFailure {
					t.Fatalf("unexpected failure to prepare config: %s", err)
				}
				t.Logf("got expected failure: %s", err)
			}

			if err == nil && tt.expectFailure {
				t.Errorf("expected failure, but prepare succeeded")
			}
		})
	}
}

func TestSerials(t *testing.T) {
	serialsTest := []struct {
		name          string
		serials       []string
		expectFailure bool
	}{
		{
			name:          "empty serials, no error",
			expectFailure: false,
			serials:       []string{},
		},
		{
			name:          "too many serials, fail",
			expectFailure: true,
			serials:       []string{"socket", "socket", "socket", "socket", "socket"},
		},
		{
			name:          "malformed serial, fail",
			expectFailure: true,
			serials:       []string{"socket", "/dev/abcde", "/mnt/device"},
		},
	}

	for _, tt := range serialsTest {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mandatoryConfig(t)
			cfg["serials"] = tt.serials

			var c Config
			_, _, err := c.Prepare(&c, cfg)
			if err != nil {
				if !tt.expectFailure {
					t.Fatalf("unexpected failure to prepare config: %s", err)
				}
				t.Logf("got expected failure: %s", err)
			}

			if err == nil && tt.expectFailure {
				t.Errorf("expected failure, but prepare succeeded")
			}
		})
	}
}

func TestVMID(t *testing.T) {
	serialsTest := []struct {
		name          string
		VMID          int
		expectFailure bool
	}{
		{
			name:          "VMID zero, no error",
			expectFailure: false,
			VMID:          0,
		},
		{
			name:          "VMID in range, no error",
			expectFailure: false,
			VMID:          1000,
		},
		{
			name:          "VMID above range, fail",
			expectFailure: true,
			VMID:          1000000000,
		},
		{
			name:          "VMID below range, fail",
			expectFailure: true,
			VMID:          50,
		},
	}

	for _, tt := range serialsTest {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mandatoryConfig(t)
			cfg["vm_id"] = tt.VMID

			var c Config
			_, _, err := c.Prepare(&c, cfg)
			if err != nil {
				if !tt.expectFailure {
					t.Fatalf("unexpected failure to prepare config: %s", err)
				}
				t.Logf("got expected failure: %s", err)
			}

			if err == nil && tt.expectFailure {
				t.Errorf("expected failure, but prepare succeeded")
			}
		})
	}
}

func TestPCIDeviceMapping(t *testing.T) {
	testCases := []struct {
		expectedToFail  bool
		pciDeviceConfig pciDeviceConfig
		machine         string
		vga             vgaConfig
	}{
		// Host PCI ID / Mapping
		{
			expectedToFail: true,
			// This test fails because we haven't specified either of the two required
			// keys - `host` or `mapping`.
		},
		{
			expectedToFail: false,
			pciDeviceConfig: pciDeviceConfig{
				Host: "0000:03:00.0",
			},
		},
		{
			expectedToFail: false,
			pciDeviceConfig: pciDeviceConfig{
				Mapping: "someNic",
			},
		},
		// Legacy IGD
		{
			expectedToFail: true,
			pciDeviceConfig: pciDeviceConfig{
				LegacyIGD: boolPtr(true),
			},
		},
		{
			expectedToFail: true,
			pciDeviceConfig: pciDeviceConfig{
				LegacyIGD: boolPtr(true),
			},
			machine: "pc",
		},
		{
			expectedToFail: true,
			pciDeviceConfig: pciDeviceConfig{
				Host:      "0000:03:00.0",
				LegacyIGD: boolPtr(true),
			},
			machine: "pc",
			vga: vgaConfig{
				Type: "std",
			},
		},
		{
			expectedToFail: false,
			pciDeviceConfig: pciDeviceConfig{
				Host:      "0000:03:00.0",
				LegacyIGD: boolPtr(true),
			},
			machine: "pc",
			vga: vgaConfig{
				Type: "none",
			},
		},
		// PCIe
		{
			expectedToFail: true,
			pciDeviceConfig: pciDeviceConfig{
				Host: "0000:03:00.0",
				PCIe: boolPtr(true),
			},
			machine: "pc",
		},
		{
			expectedToFail: false,
			pciDeviceConfig: pciDeviceConfig{
				Host: "0000:03:00.0",
				PCIe: boolPtr(true),
			},
			machine: "q35",
		},
		{
			expectedToFail: false,
			pciDeviceConfig: pciDeviceConfig{
				Host: "0000:03:00.0",
				PCIe: boolPtr(true),
			},
			machine: "pc-q35",
		},
	}

	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("%d,expected_to_fail:%t", idx, tc.expectedToFail), func(t *testing.T) {
			device := make(map[string]interface{})
			device["host"] = tc.pciDeviceConfig.Host
			device["device_id"] = tc.pciDeviceConfig.DeviceID
			device["legacy_igd"] = tc.pciDeviceConfig.LegacyIGD
			device["mapping"] = tc.pciDeviceConfig.Mapping
			device["pcie"] = tc.pciDeviceConfig.PCIe
			device["mdev"] = tc.pciDeviceConfig.MDEV
			device["rombar"] = tc.pciDeviceConfig.ROMBar
			device["romfile"] = tc.pciDeviceConfig.ROMFile
			device["sub_device_id"] = tc.pciDeviceConfig.SubDeviceID
			device["sub_vendor_id"] = tc.pciDeviceConfig.SubVendorID
			device["vendor_id"] = tc.pciDeviceConfig.VendorID
			device["x_vga"] = tc.pciDeviceConfig.XVGA

			devices := make([]map[string]interface{}, 0)
			devices = append(devices, device)

			cfg := mandatoryConfig(t)
			cfg["pci_devices"] = devices
			cfg["vga"] = map[string]interface{}{"type": tc.vga.Type}
			cfg["machine"] = tc.machine

			var c Config
			_, _, err := c.Prepare(&c, cfg)

			if tc.expectedToFail == true && err == nil {
				t.Error("expected config preparation to fail, but no error occured")
			}

			if tc.expectedToFail == false && err != nil {
				t.Errorf("expected config preparation to succeed, but %s", err.Error())
			}
		})
	}
}
