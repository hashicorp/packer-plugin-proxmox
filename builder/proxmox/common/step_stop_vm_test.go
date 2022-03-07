package proxmox

import (
	"context"
	"fmt"
	"testing"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type vmTerminatorMock struct {
	shutdownVm func(*proxmox.VmRef) (string, error)
}

func (m vmTerminatorMock) ShutdownVm(r *proxmox.VmRef) (string, error) {
	return m.shutdownVm(r)
}

var _ vmTerminator = vmTerminatorMock{}

func TestStopVM(t *testing.T) {
	cs := []struct {
		name                 string
		shutdownErr          error
		expectCallShutdownVm bool
		expectedAction       multistep.StepAction
	}{
		{
			name:                 "NoError",
			expectCallShutdownVm: true,
			expectedAction:       multistep.ActionContinue,
		},
		{
			name:                 "RaiseShutdownVMError",
			expectCallShutdownVm: true,
			shutdownErr:          fmt.Errorf("failed to stop vm"),
			expectedAction:       multistep.ActionHalt,
		},
	}

	const vmid = 123

	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			terminator := vmTerminatorMock{
				shutdownVm: func(r *proxmox.VmRef) (string, error) {
					if r.VmId() != vmid {
						t.Errorf("ShutdownVm called with unexpected id, expected %d, got %d", vmid, r.VmId())
					}
					if !c.expectCallShutdownVm {
						t.Error("Did not expect ShutdownVM to be called")
					}
					return "", c.shutdownErr
				},
			}

			state := new(multistep.BasicStateBag)
			state.Put("ui", packersdk.TestUi(t))
			state.Put("vmRef", proxmox.NewVmRef(vmid))
			state.Put("proxmoxClient", terminator)

			step := stepStopVM{}
			action := step.Run(context.TODO(), state)
			if action != c.expectedAction {
				t.Errorf("Expected action to be %v, got %v", c.expectedAction, action)
			}
		})
	}
}
