See [Releases](https://github.com/hashicorp/packer-plugin-proxmox/releases) for latest CHANGELOG information.

## 1.0.1 (June 14, 2021)

* Allow user to specify task_timeout [GH-#2]

## 1.0.0 (June 14, 2021)

### Enhancements:

* Validate template_name and vm_name per proxmox requirements. [GH-15]
* Update to latest Packer Plugin SDK [GH-19]

### Bug Fixes:

*  Fix qemu_agent to default to true when using HCL2 [GH-17]

## 0.0.2 (April 20, 2021)

* Fast-follow release to resolve goreleaser issues.

## 0.0.1 (April 20, 2021)

* Proxmox Plugin break out from Packer core. Changes prior to break out can be found in [Packer's CHANGELOG](https://github.com/hashicorp/packer/blob/master/CHANGELOG.md).
