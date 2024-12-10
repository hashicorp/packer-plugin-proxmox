Type: `proxmox-virtualmachine`
Artifact BuilderId: `proxmox.virtualmachine`

The `proxmox-virtualmachine` datasource retrieves information about existing virtual machines
from [Proxmox](https://www.proxmox.com/en/proxmox-ve) cluster and returns VM ID of one virtual machine
that matches all specified filters. This ID can be used in the `proxmox-clone` builder to select a template.

## Configuration Reference

<!-- Code generated from the comments of the Config struct in datasource/virtualmachine/data.go; DO NOT EDIT MANUALLY -->

Datasource has a bunch of filters which you can use, for example, to find the latest available
template in the cluster that matches defined filters.

You can combine any number of filters but all of them will be conjuncted with AND.
When datasource cannot return only one (zero or >1) guest identifiers it will return error.

<!-- End of code generated from the comments of the Config struct in datasource/virtualmachine/data.go; -->


## Optional:

<!-- Code generated from the comments of the Config struct in datasource/virtualmachine/data.go; DO NOT EDIT MANUALLY -->

- `proxmox_url` (string) - URL to the Proxmox API, including the full path,
  so `https://<server>:<port>/api2/json` for example.
  Can also be set via the `PROXMOX_URL` environment variable.

- `insecure_skip_tls_verify` (bool) - Skip validating the certificate.

- `username` (string) - Username when authenticating to Proxmox, including
  the realm. For example `user@pve` to use the local Proxmox realm. When using
  token authentication, the username must include the token id after an exclamation
  mark. For example, `user@pve!tokenid`.
  Can also be set via the `PROXMOX_USERNAME` environment variable.

- `password` (string) - Password for the user.
  For API tokens please use `token`.
  Can also be set via the `PROXMOX_PASSWORD` environment variable.
  Either `password` or `token` must be specifed. If both are set,
  `token` takes precedence.

- `token` (string) - Token for authenticating API calls.
  This allows the API client to work with API tokens instead of user passwords.
  Can also be set via the `PROXMOX_TOKEN` environment variable.
  Either `password` or `token` must be specifed. If both are set,
  `token` takes precedence.

- `task_timeout` (duration string | ex: "1h5m2s") - `task_timeout` (duration string | ex: "10m") - The timeout for
   Promox API operations, e.g. clones. Defaults to 1 minute.

- `name` (string) - Filter that returns `vm_id` for virtual machine which name exactly matches this value.
  Options `name` and `name_regex` are mutually exclusive.

- `name_regex` (string) - Filter that returns `vm_id` for virtual machine which name matches the regular expression.
  Expression must use [Go Regex Syntax](https://pkg.go.dev/regexp/syntax).
  Options `name` and `name_regex` are mutually exclusive.

- `template` (bool) - Filter that returns virtual machine `vm_id` only when virtual machine is template.

- `node` (string) - Filter that returns `vm_id` only when virtual machine is located on the specified PVE node.

- `vm_tags` (string) - Filter that returns `vm_id` for virtual machine which has all these tags. When you need to
  specify more than one tag, use semicolon as separator (`"tag1;tag2"`).
  Every specified tag must exist in virtual machine.

- `latest` (bool) - This filter determines how to handle multiple virtual machines that were matched with all
  previous filters. Virtual machine creation time is being used to find latest.
  By default, multiple matching virtual machines results in an error.

<!-- End of code generated from the comments of the Config struct in datasource/virtualmachine/data.go; -->


## Output:

<!-- Code generated from the comments of the DatasourceOutput struct in datasource/virtualmachine/data.go; DO NOT EDIT MANUALLY -->

- `vm_id` (uint) - Identifier of the found virtual machine.

- `vm_name` (string) - Name of the found virtual machine.

- `vm_tags` (string) - Tags of the found virtual machine separated with semicolon.

<!-- End of code generated from the comments of the DatasourceOutput struct in datasource/virtualmachine/data.go; -->


## Example Usage

This is a very basic example which connects to local PVE host, finds the latest
guest which name matches the regex `image-.*` and which type is `template`. The
ID of the virtual machine is printed to console as output variable.

```hcl
variable "password" {
  type    = string
  default = "supersecret"
}

variable "username" {
  type    = string
  default = "apiuser@pve"
}

data "proxmox-virtualmachine" "default" {
    proxmox_url = "https://my-proxmox.my-domain:8006/api2/json"
    insecure_skip_tls_verify = true
    username = "${var.username}"
    password = "${var.password}"
    name_regex = "image-.*"
    template = true
    latest   = true
}

locals {
  vm_id = data.proxmox-virtualmachine.default.vm_id
}

source "null" "basic-example" {
    communicator = "none"
}

build {
  sources = [
    "source.null.basic-example"
  ]

  provisioner "shell-local" {
    inline = [
      "echo vm_id: ${local.vm_id}",
    ]
  }
}
```
