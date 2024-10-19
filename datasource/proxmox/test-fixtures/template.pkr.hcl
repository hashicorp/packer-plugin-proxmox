data "proxmox-template" "default" {
    proxmox_url = "https://localhost:8006/api2/json"
    insecure_skip_tls_verify = true
    username = "root@pam"
    password = "password"
    latest   = true
}

locals {
  vm_id = data.proxmox-template.default.vm_id
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
