package proxmox

import (
	"context"
	"fmt"
	"testing"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type startedVMCleanerMock struct {
	stopVm   func() (string, error)
	deleteVm func() (string, error)
}

func (m startedVMCleanerMock) StopVm(*proxmox.VmRef) (string, error) {
	return m.stopVm()
}
func (m startedVMCleanerMock) DeleteVm(*proxmox.VmRef) (string, error) {
	return m.deleteVm()
}

var _ startedVMCleaner = &startedVMCleanerMock{}

func TestCleanupStartVM(t *testing.T) {
	cs := []struct {
		name               string
		setVmRef           bool
		setSuccess         bool
		stopVMErr          error
		expectCallStopVM   bool
		deleteVMErr        error
		expectCallDeleteVM bool
	}{
		{
			name:             "when vmRef state is not set, nothing should happen",
			setVmRef:         false,
			expectCallStopVM: false,
		},
		{
			name:             "when success state is set, nothing should happen",
			setVmRef:         true,
			setSuccess:       true,
			expectCallStopVM: false,
		},
		{
			name:               "when not successful, vm should be stopped and deleted",
			setVmRef:           true,
			setSuccess:         false,
			expectCallStopVM:   true,
			expectCallDeleteVM: true,
		},
		{
			name:               "if stopping fails, DeleteVm should not be called",
			setVmRef:           true,
			setSuccess:         false,
			expectCallStopVM:   true,
			stopVMErr:          fmt.Errorf("some error"),
			expectCallDeleteVM: false,
		},
	}

	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			var stopWasCalled, deleteWasCalled bool

			cleaner := startedVMCleanerMock{
				stopVm: func() (string, error) {
					if !c.expectCallStopVM {
						t.Error("Did not expect StopVm to be called")
					}

					stopWasCalled = true
					return "", c.stopVMErr
				},
				deleteVm: func() (string, error) {
					if !c.expectCallDeleteVM {
						t.Error("Did not expect DeleteVm to be called")
					}

					deleteWasCalled = true
					return "", c.deleteVMErr
				},
			}

			state := new(multistep.BasicStateBag)
			state.Put("ui", packersdk.TestUi(t))
			state.Put("proxmoxClient", cleaner)
			if c.setVmRef {
				state.Put("vmRef", proxmox.NewVmRef(1))
			}
			if c.setSuccess {
				state.Put("success", "true")
			}

			step := stepStartVM{}
			step.Cleanup(state)

			if c.expectCallStopVM && !stopWasCalled {
				t.Error("Expected StopVm to be called, but it wasn't")
			}
			if c.expectCallDeleteVM && !deleteWasCalled {
				t.Error("Expected DeleteVm to be called, but it wasn't")
			}
		})
	}
}

type startVMMock struct {
	create      func(*proxmox.VmRef, proxmox.ConfigQemu, multistep.StateBag) error
	startVm     func(*proxmox.VmRef) (string, error)
	setVmConfig func(*proxmox.VmRef, map[string]interface{}) (interface{}, error)
	getNextID   func(id int) (int, error)
}

func (m *startVMMock) Create(vmRef *proxmox.VmRef, config proxmox.ConfigQemu, state multistep.StateBag) error {
	return m.create(vmRef, config, state)
}
func (m *startVMMock) StartVm(vmRef *proxmox.VmRef) (string, error) {
	return m.startVm(vmRef)
}
func (m *startVMMock) SetVmConfig(vmRef *proxmox.VmRef, config map[string]interface{}) (interface{}, error) {
	return m.setVmConfig(vmRef, config)
}
func (m *startVMMock) GetNextID(id int) (int, error) {
	return m.getNextID(id)
}

func TestStartVM(t *testing.T) {
	// TODO: proxmox-api-go does a lot of manipulation on the input and does not
	// give any way to access the actual data it sends to the Proxmox server,
	// which means writing good tests here is quite hard. This test is mainly a
	// stub to revisit when we can write better tests.
	cs := []struct {
		name           string
		config         *Config
		expectedAction multistep.StepAction
	}{
		{
			name: "Example config from documentation works",
			config: &Config{
				Disks: []diskConfig{
					{
						Type:            "sata",
						Size:            "10G",
						StoragePool:     "local",
						StoragePoolType: "lvm",
					},
				},
				NICs: []nicConfig{
					{
						Bridge: "vmbr0",
					},
				},
			},
			expectedAction: multistep.ActionContinue,
		},
	}

	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			mock := &startVMMock{
				create: func(vmRef *proxmox.VmRef, config proxmox.ConfigQemu, state multistep.StateBag) error {
					return nil
				},
				startVm: func(*proxmox.VmRef) (string, error) {
					return "", nil
				},
				setVmConfig: func(*proxmox.VmRef, map[string]interface{}) (interface{}, error) {
					return nil, nil
				},
				getNextID: func(id int) (int, error) {
					return 1, nil
				},
			}
			state := new(multistep.BasicStateBag)
			state.Put("ui", packersdk.TestUi(t))
			state.Put("config", c.config)
			state.Put("proxmoxClient", mock)
			s := stepStartVM{vmCreator: mock}

			action := s.Run(context.TODO(), state)
			if action != c.expectedAction {
				t.Errorf("Expected action %s, got %s", c.expectedAction, action)
			}
		})
	}
}

func TestStartVMRetryOnDuplicateID(t *testing.T) {
	newDuplicateError := func(id int) error {
		return fmt.Errorf("unable to create VM %d - VM %d already exists on node 'test'", id, id)
	}
	cs := []struct {
		name                  string
		config                *Config
		createErrorGenerator  func(id int) error
		expectedCallsToCreate int
		expectedAction        multistep.StepAction
	}{
		{
			name:                  "Succeed immediately if non-duplicate",
			config:                &Config{},
			createErrorGenerator:  func(id int) error { return nil },
			expectedCallsToCreate: 1,
			expectedAction:        multistep.ActionContinue,
		},
		{
			name:                  "Fail immediately if duplicate and VMID explicitly configured",
			config:                &Config{VMID: 1},
			createErrorGenerator:  func(id int) error { return newDuplicateError(id) },
			expectedCallsToCreate: 1,
			expectedAction:        multistep.ActionHalt,
		},
		{
			name:                  "Fail immediately if error not caused by duplicate ID",
			config:                &Config{},
			createErrorGenerator:  func(id int) error { return fmt.Errorf("Something else went wrong") },
			expectedCallsToCreate: 1,
			expectedAction:        multistep.ActionHalt,
		},
		{
			name:   "Retry if error caused by duplicate ID",
			config: &Config{},
			createErrorGenerator: func(id int) error {
				if id < 2 {
					return newDuplicateError(id)
				}
				return nil
			},
			expectedCallsToCreate: 2,
			expectedAction:        multistep.ActionContinue,
		},
		{
			name:   "Retry only up to maxDuplicateIDRetries times",
			config: &Config{},
			createErrorGenerator: func(id int) error {
				return newDuplicateError(id)
			},
			expectedCallsToCreate: maxDuplicateIDRetries,
			expectedAction:        multistep.ActionHalt,
		},
	}

	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			createCalls := 0
			mock := &startVMMock{
				create: func(vmRef *proxmox.VmRef, config proxmox.ConfigQemu, state multistep.StateBag) error {
					createCalls++
					return c.createErrorGenerator(vmRef.VmId())
				},
				startVm: func(*proxmox.VmRef) (string, error) {
					return "", nil
				},
				setVmConfig: func(*proxmox.VmRef, map[string]interface{}) (interface{}, error) {
					return nil, nil
				},
				getNextID: func(id int) (int, error) {
					return createCalls + 1, nil
				},
			}
			state := new(multistep.BasicStateBag)
			state.Put("ui", packersdk.TestUi(t))
			state.Put("config", c.config)
			state.Put("proxmoxClient", mock)
			s := stepStartVM{vmCreator: mock}

			action := s.Run(context.TODO(), state)
			if action != c.expectedAction {
				t.Errorf("Expected action %s, got %s", c.expectedAction, action)
			}
			if createCalls != c.expectedCallsToCreate {
				t.Errorf("Expected %d calls to create, got %d", c.expectedCallsToCreate, createCalls)
			}
		})
	}
}
