package scaffolding

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// This is a definition of a builder step and should implement multistep.Step
type StepSayConfig struct {
	MockConfig string
}

// Run should execute the purpose of this step
func (s *StepSayConfig) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	if s.MockConfig == "" {
		ui.Error("'mock' should be set to say something.")
		// Errors should be added to the state to be check it out
		// later at the end of the build
		state.Put("error", fmt.Errorf("'mock' not set"))
		// Determines that the build steps should be halted
		return multistep.ActionHalt
	}

	ui.Say(fmt.Sprintf("The mock config is set to %q", s.MockConfig))
	// Determines that should continue to the next step
	return multistep.ActionContinue
}

// Cleanup can be used to clean up any artifact created by the step.
// A step's clean up always run at the end of a build, regardless of whether provisioning succeeds or fails.
func (s *StepSayConfig) Cleanup(_ multistep.StateBag) {
	// Nothing to clean
}
