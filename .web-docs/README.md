The Proxmox Packer builder is able to create [Proxmox](https://www.proxmox.com/en/proxmox-ve) virtual machines and store them as new Proxmox Virtual Machine images.

### Installation

To install this plugin add this code into your Packer configuration and run [packer init](/packer/docs/commands/init)

```hcl
packer {
  required_plugins {
    name = {
      version = "~> 1"
      source  = "github.com/hashicorp/proxmox"
    }
  }
}
```
Alternatively, you can use `packer plugins install` to manage installation of this plugin.

```sh
packer plugins install github.com/hashicorp/proxmox
```

### Components

Packer is able to target both ISO and existing Cloud-Init images.

#### Builders

- [proxmox-clone](/packer/integrations/hashicorp/proxmox/latest/components/builder/clone) - The proxmox Packer
  builder is able to create new images for use with Proxmox VE. The builder
  takes an ISO source, runs any provisioning necessary on the image after
  launching it, then creates a virtual machine template.
- [proxmox-iso](/packer/integrations/hashicorp/proxmox/latest/components/builder/iso) - The proxmox Packer
  builder is able to create new images for use with Proxmox VE. The builder
  takes an ISO source, runs any provisioning necessary on the image after
  launching it, then creates a virtual machine template.

