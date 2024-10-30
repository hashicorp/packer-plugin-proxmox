// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

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
	artifactID    int
	artifactType  string
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
	return strconv.Itoa(a.artifactID)
}

func (a *Artifact) String() string {
	return fmt.Sprintf("A %s was created: %d", a.artifactType, a.artifactID)
}

func (a *Artifact) State(name string) interface{} {
	return a.StateData[name]
}

func (a *Artifact) Destroy() error {
	log.Printf("Destroying %s: %d", a.artifactType, a.artifactID)
	_, err := a.proxmoxClient.DeleteVm(proxmox.NewVmRef(a.artifactID))
	return err
}
