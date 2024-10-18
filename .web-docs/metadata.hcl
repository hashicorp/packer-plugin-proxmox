# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# For full specification on the configuration of this file visit:
# https://github.com/hashicorp/integration-template#metadata-configuration
integration {
  name = "Proxmox"
  description = "The Proxmox Packer builder is able to create Proxmox virtual machines and store them as new Proxmox Virtual Machine images."
  identifier = "packer/hashicorp/proxmox"
  component {
    type = "builder"
    name = "Proxmox Clone"
    slug = "clone"
  }
  component {
    type = "builder"
    name = "Proxmox ISO"
    slug = "iso"
  }
  component {
    type = "data-source"
    name = "Proxmox Template"
    slug = "template"
  }
}
