data "proxmox-template" "default" {
    proxmox_url = "https://localhost:8006/api2/json"
    insecure_skip_tls_verify = true
    username = "root@pam"
    password = "password"
}

locals {
  foo = data.proxmox-template.default.foo
  bar = data.proxmox-template.default.bar
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
      "echo foo: ${local.foo}",
      "echo bar: ${local.bar}",
    ]
  }
}
