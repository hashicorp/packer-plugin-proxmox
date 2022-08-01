package proxmox

import (
	"context"
	"fmt"
	"testing"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

type converterMock struct {
	createTemplate func(*proxmox.VmRef) error
}

func (m converterMock) CreateTemplate(r *proxmox.VmRef) error {
	return m.createTemplate(r)
}

var _ templateConverter = converterMock{}

func TestConvertToTemplate(t *testing.T) {
	cs := []struct {
		name                     string
		expectCallCreateTemplate bool
		createTemplateErr        error
		expectedAction           multistep.StepAction
		builderConfig            *Config
	}{
		{
			name:                     "NoErrorsUnset",
			expectCallCreateTemplate: true,
			createTemplateErr:        nil,
			expectedAction:           multistep.ActionContinue,
			builderConfig:            &Config{},
		},
		{
			name:                     "NoErrors",
			expectCallCreateTemplate: true,
			createTemplateErr:        nil,
			expectedAction:           multistep.ActionContinue,
			builderConfig: &Config{
				SkipConvertToTemplate: config.TriFalse,
			},
		},
		{
			name:                     "RaiseConvertTemplateError",
			expectCallCreateTemplate: true,
			createTemplateErr:        fmt.Errorf("failed to convert vm to template"),
			expectedAction:           multistep.ActionHalt,
			builderConfig: &Config{
				SkipConvertToTemplate: config.TriFalse,
			},
		},
		{
			name:                     "SkipConvertToTemplate",
			expectCallCreateTemplate: false,
			createTemplateErr:        nil,
			expectedAction:           multistep.ActionContinue,
			builderConfig: &Config{
				SkipConvertToTemplate: config.TriTrue,
			},
		},
	}

	const vmid = 1

	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			converter := converterMock{
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

		})
	}
}
