// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"fmt"
	"regexp"
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

func TestISOs(t *testing.T) {
	isotests := []struct {
		name           string
		expectedToFail bool
		ISOs           map[string]interface{}
	}{
		{
			name:           "missing ISO definition should error",
			expectedToFail: true,
			ISOs: map[string]interface{}{
				"type": "ide",
			},
		},
		{
			name:           "cd_files and iso_file specified should fail",
			expectedToFail: true,
			ISOs: map[string]interface{}{
				"type": "ide",
				"cd_files": []string{
					"config_test.go",
				},
				"iso_file": "local:iso/test.iso",
			},
		},
		{
			name:           "cd_files, iso_file and iso_url specified should fail",
			expectedToFail: true,
			ISOs: map[string]interface{}{
				"type": "ide",
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
			ISOs: map[string]interface{}{
				"type": "ide",
				"cd_files": []string{
					"config_test.go",
				},
			},
		},
		{
			name:           "cd_files valid should succeed",
			expectedToFail: false,
			ISOs: map[string]interface{}{
				"type": "ide",
				"cd_files": []string{
					"config_test.go",
				},
				"iso_storage_pool": "local",
			},
		},
		{
			name:           "cd_content valid should succeed",
			expectedToFail: false,
			ISOs: map[string]interface{}{
				"type": "ide",
				"cd_content": map[string]string{
					"test": "config_test.go",
				},
				"iso_storage_pool": "local",
			},
		},
		{
			name:           "iso_url valid should succeed",
			expectedToFail: false,
			ISOs: map[string]interface{}{
				"type":             "ide",
				"iso_url":          "http://example.com",
				"iso_checksum":     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"iso_storage_pool": "local",
			},
		},
		{
			name:           "iso_file valid should succeed",
			expectedToFail: false,
			ISOs: map[string]interface{}{
				"type":     "ide",
				"iso_file": "local:iso/test.iso",
			},
		},
	}

	for _, c := range isotests {
		t.Run(c.name, func(t *testing.T) {
			cfg := mandatoryConfig(t)
			cfg["additional_iso_files"] = c.ISOs

			var config Config
			_, _, err := config.Prepare(&config, cfg)

			if c.expectedToFail && err == nil {
				t.Error("expected config preparation to fail, but no error occured")
			}

			if !c.expectedToFail && err != nil {
				t.Errorf("expected config preparation to succeed, but %s", err.Error())
			}
		})
	}
}

func TestDeprecatedISOOptionsAreConverted(t *testing.T) {
	isotests := []struct {
		name           string
		expectedToFail bool
		ISOs           map[string]interface{}
	}{
		{
			name:           "cd_files valid should succeed",
			expectedToFail: false,
			ISOs: map[string]interface{}{
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
			ISOs: map[string]interface{}{
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
			ISOs: map[string]interface{}{
				"device":           "ide1",
				"iso_url":          "http://example.com",
				"iso_checksum":     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"iso_storage_pool": "local",
			},
		},
		{
			name:           "iso_file valid should succeed",
			expectedToFail: false,
			ISOs: map[string]interface{}{
				"device":   "ide1",
				"iso_file": "local:iso/test.iso",
			},
		},
	}
	for _, c := range isotests {
		t.Run(c.name, func(t *testing.T) {
			cfg := mandatoryConfig(t)
			cfg["additional_iso_files"] = c.ISOs

			var config Config
			_, _, err := config.Prepare(&config, cfg)
			if err != nil {
				t.Fatal(err)
			}

			rd := regexp.MustCompile(`\D+`)
			bus := rd.FindString(config.ISOs[0].Device)

			rb := regexp.MustCompile(`\d+`)
			index := rb.FindString(config.ISOs[0].Device)

			if config.ISOs[0].Type != bus {
				t.Errorf("Expected device to be converted to type %s", bus)
			}
			if config.ISOs[0].Index != index {
				t.Errorf("Expected device to be converted to index %s", index)
			}

			if c.expectedToFail && err == nil {
				t.Error("expected config preparation to fail, but no error occured")
			}

			if !c.expectedToFail && err != nil {
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

func TestTpm(t *testing.T) {
	TpmTest := []struct {
		name          string
		tpm_config    tpmConfig
		expectFailure bool
	}{
		{
			name:          "version 1.2, no error",
			expectFailure: false,
			tpm_config: tpmConfig{
				TPMStoragePool: "local",
				Version:        "v1.2",
			},
		},
		{
			name:          "version 2.0, no error",
			expectFailure: false,
			tpm_config: tpmConfig{
				TPMStoragePool: "local",
				Version:        "v2.0",
			},
		},
		{
			name:          "empty storage pool, error",
			expectFailure: true,
			tpm_config: tpmConfig{
				TPMStoragePool: "",
				Version:        "v1.2",
			},
		},
		{
			name:          "invalid Version, error",
			expectFailure: true,
			tpm_config: tpmConfig{
				TPMStoragePool: "local",
				Version:        "v6.2",
			},
		},
	}

	for _, tt := range TpmTest {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mandatoryConfig(t)
			cfg["tpm_config"] = &tt.tpm_config

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
		expectedError   error
		pciDeviceConfig pciDeviceConfig
		machine         string
		vga             vgaConfig
	}{
		// Host PCI ID / Mapping
		{
			expectedError: fmt.Errorf("either the host or the mapping key must be specified"),
		},
		{
			expectedError: fmt.Errorf("the host and the mapping key cannot both be set"),
			pciDeviceConfig: pciDeviceConfig{
				Host:    "0000:03:00.0",
				Mapping: "someNic",
			},
		},
		{
			expectedError: fmt.Errorf("host contains invalid PCI ID"),
			pciDeviceConfig: pciDeviceConfig{
				Host: "invalid-pci-id",
			},
		},
		{
			pciDeviceConfig: pciDeviceConfig{
				Host: "0000:03:00.0",
			},
		},
		{
			pciDeviceConfig: pciDeviceConfig{
				Mapping: "someNic",
			},
		},
		// Legacy IGD
		{
			expectedError: fmt.Errorf("legacy_igd requires pc-i440fx machine type"),
			pciDeviceConfig: pciDeviceConfig{
				Host:      "0000:03:00.0",
				LegacyIGD: true,
			},
		},
		{
			expectedError: fmt.Errorf("legacy_igd requires vga.type set to none"),
			pciDeviceConfig: pciDeviceConfig{
				Host:      "0000:03:00.0",
				LegacyIGD: true,
			},
			machine: "pc",
		},
		{
			expectedError: fmt.Errorf("legacy_igd requires vga.type set to none"),
			pciDeviceConfig: pciDeviceConfig{
				Host:      "0000:03:00.0",
				LegacyIGD: true,
			},
			machine: "pc",
			vga: vgaConfig{
				Type: "std",
			},
		},
		{
			pciDeviceConfig: pciDeviceConfig{
				Host:      "0000:03:00.0",
				LegacyIGD: true,
			},
			machine: "pc",
			vga: vgaConfig{
				Type: "none",
			},
		},
		// PCIe
		{
			expectedError: fmt.Errorf("pcie requires q35 machine type"),
			pciDeviceConfig: pciDeviceConfig{
				Host: "0000:03:00.0",
				PCIe: true,
			},
			machine: "pc",
		},
		{
			pciDeviceConfig: pciDeviceConfig{
				Host: "0000:03:00.0",
				PCIe: true,
			},
			machine: "q35",
		},
		{
			pciDeviceConfig: pciDeviceConfig{
				Host: "0000:03:00.0",
				PCIe: true,
			},
			machine: "pc-q35",
		},
	}

	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("%d,expected_to_fail:%t", idx, tc.expectedError != nil), func(t *testing.T) {
			device := make(map[string]interface{})
			device["host"] = tc.pciDeviceConfig.Host
			device["device_id"] = tc.pciDeviceConfig.DeviceID
			device["legacy_igd"] = tc.pciDeviceConfig.LegacyIGD
			device["mapping"] = tc.pciDeviceConfig.Mapping
			device["pcie"] = tc.pciDeviceConfig.PCIe
			device["mdev"] = tc.pciDeviceConfig.MDEV
			device["hide_rombar"] = tc.pciDeviceConfig.HideROMBAR
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

			switch {
			case tc.expectedError == nil && err != nil:
				t.Errorf("expected config preparation to succeed, but %s", err.Error())
			case tc.expectedError != nil && err == nil:
				t.Error("expected config preparation to fail, but no error occured")
			case tc.expectedError != nil && !strings.Contains(err.Error(), tc.expectedError.Error()):
				t.Errorf("expected config preparation errors to match - want %q, got %q", tc.expectedError, err)
			}
		})
	}
}
