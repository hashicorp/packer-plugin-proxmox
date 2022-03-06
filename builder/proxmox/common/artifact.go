package proxmox

import (
	"fmt"
	"log"
	"strconv"

	"github.com/Telmate/proxmox-api-go/proxmox"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type Artifact struct {
	builderID     string
	vmID          int
	isTemplate    bool
	proxmoxClient *proxmox.Client

	// StateData should store data such as GeneratedData
	// to be shared with post-processors
	StateData map[string]interface{}
}

// Artifact implements packersdk.Artifact
var _ packersdk.Artifact = &Artifact{}

func (a *Artifact) BuilderId() string {
	return a.builderID
}

func (*Artifact) Files() []string {
	return nil
}

func (a *Artifact) Id() string {
	return strconv.Itoa(a.vmID)
}

func (a *Artifact) String() string {

	if a.isTemplate {
		return fmt.Sprintf("A Template was created: %d", a.vmID)
	}
	return fmt.Sprintf("A VM was created: %d", a.vmID)
}

func (a *Artifact) State(name string) interface{} {
	return a.StateData[name]
}

func (a *Artifact) Destroy() error {

	if a.isTemplate {
		log.Printf("Destroying Template: %d", a.vmID)
	} else {
		log.Printf("Destroying VM: %d", a.vmID)
	}
	_, err := a.proxmoxClient.DeleteVm(proxmox.NewVmRef(a.vmID))
	return err
}
