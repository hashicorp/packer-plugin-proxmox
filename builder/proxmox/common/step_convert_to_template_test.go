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
	shutdownVm     func(context.Context, *proxmox.VmRef) (string, error)
	createTemplate func(context.Context, *proxmox.VmRef) error
}

func (m converterMock) ShutdownVm(ctx context.Context, r *proxmox.VmRef) (string, error) {
	return m.shutdownVm(ctx, r)
}
func (m converterMock) CreateTemplate(ctx context.Context, r *proxmox.VmRef) error {
	return m.createTemplate(ctx, r)
}

var _ templateConverter = converterMock{}

func TestConvertToTemplate(t *testing.T) {
	cs := []struct {
		name                     string
		shutdownErr              error
		expectCallCreateTemplate bool
		createTemplateErr        error
		expectedAction           multistep.StepAction
		expectTemplateIdSet      bool
	}{
		{
			name:                     "no errors returns continue and sets template id",
			expectCallCreateTemplate: true,
			expectedAction:           multistep.ActionContinue,
			expectTemplateIdSet:      true,
		},
		{
			name:                     "when shutdown fails, don't try to create template and halt",
			shutdownErr:              fmt.Errorf("failed to stop vm"),
			expectCallCreateTemplate: false,
			expectedAction:           multistep.ActionHalt,
			expectTemplateIdSet:      false,
		},
		{
			name:                     "when create template fails, halt",
			expectCallCreateTemplate: true,
			createTemplateErr:        fmt.Errorf("failed to stop vm"),
			expectedAction:           multistep.ActionHalt,
			expectTemplateIdSet:      false,
		},
	}

	const vmid = 123

	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			converter := converterMock{
				shutdownVm: func(ctx context.Context, r *proxmox.VmRef) (string, error) {
					if r.VmId() != vmid {
						t.Errorf("ShutdownVm called with unexpected id, expected %d, got %d", vmid, r.VmId())
					}
					return "", c.shutdownErr
				},
				createTemplate: func(ctx context.Context, r *proxmox.VmRef) error {
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

			step := stepConvertToTemplate{}
			action := step.Run(context.TODO(), state)
			if action != c.expectedAction {
				t.Errorf("Expected action to be %v, got %v", c.expectedAction, action)
			}

			id, wasSet := state.GetOk("template_id")

			if c.expectTemplateIdSet != wasSet {
				t.Errorf("Expected template_id state present=%v was present=%v", c.expectTemplateIdSet, wasSet)
			}

			if c.expectTemplateIdSet && id != vmid {
				t.Errorf("Expected template_id state to be set to %d, got %v", vmid, id)
			}
		})
	}
}
