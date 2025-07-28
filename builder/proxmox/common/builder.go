// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"errors"
	"fmt"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func NewSharedBuilder(id string, config Config, preSteps []multistep.Step, postSteps []multistep.Step, vmCreator ProxmoxVMCreator) *Builder {
	return &Builder{
		id:        id,
		config:    config,
		preSteps:  preSteps,
		postSteps: postSteps,
		vmCreator: vmCreator,
	}
}

type Builder struct {
	id            string
	config        Config
	preSteps      []multistep.Step
	postSteps     []multistep.Step
	runner        multistep.Runner
	proxmoxClient *proxmox.Client
	vmCreator     ProxmoxVMCreator
}

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook, state multistep.StateBag) (packersdk.Artifact, error) {
	var err error
	b.proxmoxClient, err = newProxmoxClient(b.config)
	if err != nil {
		return nil, err
	}

	// Set up the state
	state.Put("config", &b.config)
	state.Put("proxmoxClient", b.proxmoxClient)
	state.Put("hook", hook)
	state.Put("ui", ui)

	comm := &b.config.Comm

	// Build the steps
	coreSteps := []multistep.Step{
		// Since the ISOs get added to the VM before/during this line we need to update the cloud-init data before this line.  It probably needs to be before as CD creation is done in presteps
		&stepStartVM{
			vmCreator: b.vmCreator,
		},
		// Also need to update the cloud-init data before this line
		commonsteps.HTTPServerFromHTTPConfig(&b.config.HTTPConfig),
		&stepTypeBootCommand{
			BootConfig: b.config.BootConfig,
			Ctx:        b.config.Ctx,
		},
		&communicator.StepConnect{
			Config:    comm,
			Host:      commHost((*comm).Host()),
			SSHConfig: (*comm).SSHConfigFunc(),
		},
		&commonsteps.StepProvision{},
		&commonsteps.StepCleanupTempKeys{
			Comm: comm,
		},
		&stepRemoveCloudInitDrive{},
		&stepConvertToTemplate{},
		&stepFinalizeTemplateConfig{},
		&stepSuccess{},
	}

	preSteps := []multistep.Step{
		&StepSshKeyPair{
			Debug:		b.config.PackerDebug,
			DebugKeyPath:	fmt.Sprintf("%s.pem", b.config.PackerBuildName),
		},
		&StepUpdateCloudInitSSH{},
	}
	for idx := range b.config.ISOs {
		if b.config.ISOs[idx].ISODownloadPVE {
			preSteps = append(preSteps,
				&stepDownloadISOOnPVE{
					ISO: &b.config.ISOs[idx],
				},
			)
		} else {
			preSteps = append(preSteps,
				&commonsteps.StepCreateCD{
					Files:   b.config.ISOs[idx].CDConfig.CDFiles,
					Content: b.config.ISOs[idx].CDConfig.CDContent,
					Label:   b.config.ISOs[idx].CDConfig.CDLabel,
				},
				&commonsteps.StepDownload{
					Checksum:    b.config.ISOs[idx].ISOChecksum,
					Description: "ISO",
					Extension:   b.config.ISOs[idx].TargetExtension,
					ResultKey:   b.config.ISOs[idx].DownloadPathKey,
					TargetPath:  b.config.ISOs[idx].DownloadPathKey,
					Url:         b.config.ISOs[idx].ISOUrls,
				},
				&stepUploadISO{
					ISO: &b.config.ISOs[idx],
				},
			)
		}
	}

	steps := append(preSteps, b.preSteps..., coreSteps..., b.postSteps...)
	// Run the steps
	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)
	// If there was an error, return that
	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}
	// If we were interrupted or cancelled, then just exit.
	if _, ok := state.GetOk(multistep.StateCancelled); ok {
		return nil, errors.New("build was cancelled")
	}

	// Verify that the template_id was set properly, otherwise we didn't progress through the last step
	tplID, ok := state.Get("template_id").(int)
	if !ok {
		return nil, fmt.Errorf("template ID could not be determined")
	}

	artifact := &Artifact{
		builderID:     b.id,
		templateID:    tplID,
		proxmoxClient: b.proxmoxClient,
		StateData:     map[string]interface{}{"generated_data": state.Get("generated_data")},
	}

	return artifact, nil
}

// Returns ssh_host or winrm_host (see communicator.Config.Host) config
// parameter when set, otherwise gets the host IP from running VM
func commHost(host string) func(state multistep.StateBag) (string, error) {
	if host != "" {
		return func(state multistep.StateBag) (string, error) {
			return host, nil
		}
	}
	return getVMIP
}

// Reads the first non-loopback interface's IP address from the VM.
// qemu-guest-agent package must be installed on the VM
func getVMIP(state multistep.StateBag) (string, error) {
	client := state.Get("proxmoxClient").(*proxmox.Client)
	config := state.Get("config").(*Config)
	vmRef := state.Get("vmRef").(*proxmox.VmRef)

	ifs, err := client.GetVmAgentNetworkInterfaces(vmRef)
	if err != nil {
		return "", err
	}

	if config.VMInterface != "" {
		for _, iface := range ifs {
			if config.VMInterface != iface.Name {
				continue
			}

			for _, addr := range iface.IpAddresses {
				if addr.IsLoopback() {
					continue
				}
				return addr.String(), nil
			}
			return "", fmt.Errorf("Interface %s only has loopback addresses", config.VMInterface)
		}
		return "", fmt.Errorf("Interface %s not found in VM", config.VMInterface)
	}

	for _, iface := range ifs {
		for _, addr := range iface.IpAddresses {
			if addr.IsLoopback() {
				continue
			}
			if addr.To4() == nil {
				continue
			}
			return addr.String(), nil
		}
	}

	return "", fmt.Errorf("Found no IP addresses on VM")
}
