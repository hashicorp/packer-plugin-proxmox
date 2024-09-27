// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"
	"fmt"
	"testing"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type converterMock struct {
	shutdownVm     func(*proxmox.VmRef) (string, error)
	createTemplate func(*proxmox.VmRef) error
}

func (m converterMock) ShutdownVm(r *proxmox.VmRef) (string, error) {
	return m.shutdownVm(r)
}
func (m converterMock) CreateTemplate(r *proxmox.VmRef) error {
	return m.createTemplate(r)
}

var _ templateConverter = converterMock{}

func TestConvertToTemplate(t *testing.T) {
	cs := []struct {
		name                     string
		shutdownErr              error
		expectCallCreateTemplate bool
		createTemplateErr        error
		expectedAction           multistep.StepAction
		expectArtifactIdSet      bool
		expectArtifactType       string
		builderConfig            *Config
	}{
		{
			name:                     "no errors returns continue and sets template artifact type",
			expectCallCreateTemplate: true,
			expectedAction:           multistep.ActionContinue,
			expectArtifactIdSet:      true,
			expectArtifactType:       "template",
			builderConfig:            &Config{},
		},
		{
			name:                     "no errors returns continue and sets vm artifact type",
			expectCallCreateTemplate: true,
			expectedAction:           multistep.ActionContinue,
			expectArtifactIdSet:      true,
			expectArtifactType:       "VM",
			builderConfig: &Config{
				SkipConvertToTemplate: true,
			},
		},
		{
			name:                     "when shutdown fails, don't try to create template and halt",
			shutdownErr:              fmt.Errorf("failed to stop vm"),
			expectCallCreateTemplate: false,
			expectedAction:           multistep.ActionHalt,
			expectArtifactIdSet:      false,
			expectArtifactType:       "",
			builderConfig:            &Config{},
		},
		{
			name:                     "when create template fails, halt",
			expectCallCreateTemplate: true,
			createTemplateErr:        fmt.Errorf("failed to stop vm"),
			expectedAction:           multistep.ActionHalt,
			expectArtifactIdSet:      false,
			expectArtifactType:       "",
			builderConfig:            &Config{},
		},
	}

	const vmid = 123

	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			converter := converterMock{
				shutdownVm: func(r *proxmox.VmRef) (string, error) {
					if r.VmId() != vmid {
						t.Errorf("ShutdownVm called with unexpected id, expected %d, got %d", vmid, r.VmId())
					}
					return "", c.shutdownErr
				},
				createTemplate: func(r *proxmox.VmRef) error {
					if r.VmId() != vmid {
						t.Errorf("CreateTemplate called with unexpected id, expected %d, got %d", vmid, r.VmId())
					}
					if !c.expectCallCreateTemplate {
						t.Error("Did not expect CreateTemplate to be called")
					}

					return c.createTemplateErr
				},
			}

			state := new(multistep.BasicStateBag)
			state.Put("ui", packersdk.TestUi(t))
			state.Put("vmRef", proxmox.NewVmRef(vmid))
			state.Put("proxmoxClient", converter)
			state.Put("config", c.builderConfig)

			step := stepConvertToTemplate{}
			action := step.Run(context.TODO(), state)
			if action != c.expectedAction {
				t.Errorf("Expected action to be %v, got %v", c.expectedAction, action)
			}

			artifactId, artifactIdWasSet := state.GetOk("artifact_id")
			artifactType, artifactTypeWasSet := state.GetOk("artifact_type")

			if c.expectArtifactIdSet != artifactIdWasSet {
				t.Errorf("Expected artifact_id state present=%v was present=%v", c.expectArtifactIdSet, artifactIdWasSet)
			}

			if c.expectArtifactIdSet && artifactId != vmid {
				t.Errorf("Expected artifact_id state to be set to %d, got %v", vmid, artifactId)
			}

			if c.expectArtifactType == "" && artifactTypeWasSet {
				t.Errorf("Expected artifact_type state present=%v was present=%v", c.expectArtifactType, artifactTypeWasSet)
			}

			if artifactTypeWasSet && c.expectArtifactType != artifactType {
				t.Errorf("Expected artifact_type state to be set to %s, got %s", c.expectArtifactType, artifactType)
			}
		})
	}
}
