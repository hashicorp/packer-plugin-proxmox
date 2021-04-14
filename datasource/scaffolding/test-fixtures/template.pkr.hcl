data "scaffolding-my-datasource" "test" {
  mock = "mock-config"
}

locals {
  foo = data.scaffolding-my-datasource.test.foo
  bar = data.scaffolding-my-datasource.test.bar
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
