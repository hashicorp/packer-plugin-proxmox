# Packer Plugin Scaffolding

This repository is a template for a Packer multi-component plugin. It is intended as a starting point for creating Packer plugins, containing:
- A builder ([builder/scaffolding](builder/scaffolding))
- A provisioner ([provisioner/scaffolding](provisioner/scaffolding))
- A post-processor ([post-processor/scaffolding](provisioner/scaffolding))
- A data source ([datasource/scaffolding](datasource/scaffolding))
- Docs ([docs](docs))
- A working example ([example](example))

These folders contain boilerplate code that you will need to edit to create your own Packer multi-component plugin.
A full guide to creating Packer plugins can be found at [Extending Packer](https://www.packer.io/docs/plugins/creation).

In this repository you will also find a pre-defined GitHub Action configuration for the release workflow
(`.goreleaser.yml` and `.github/workflows/release.yml`). The release workflow configuration makes sure the GitHub
release artifacts are created with the correct binaries and naming conventions.

Please see the [GitHub template repository documentation](https://docs.github.com/en/free-pro-team@latest/github/creating-cloning-and-archiving-repositories/creating-a-repository-from-a-template)
for how to create a new repository from this template on GitHub.

## Packer plugin projects

Here's a non exaustive list of Packer plugins that you can checkout:

* [github.com/hashicorp/packer-plugin-docker](https://github.com/hashicorp/packer-plugin-docker)
* [github.com/exoscale/packer-plugin-exoscale](https://github.com/exoscale/packer-plugin-exoscale)
* [github.com/sylviamoss/packer-plugin-comment](https://github.com/sylviamoss/packer-plugin-comment)
* [github.com/hashicorp/packer-plugin-hashicups](https://github.com/hashicorp/packer-plugin-hashicups)

Looking at their code will give you good examples.

## Running Acceptance Tests

Make sure to install the plugin with `go build .` and to have Packer installed locally.
Then source the built binary to the plugin path with `cp packer-plugin-scaffolding ~/.packer.d/plugins/packer-plugin-scaffolding`
Once everything needed is set up, run:
```
PACKER_ACC=1 go test -count 1 -v ./... -timeout=120m
```

This will run the acceptance tests for all plugins in this set.

## Test Plugin Example Action

This scaffolding configures a [manually triggered plugin test action](/.github/workflows/test-plugin-example.yml).
By default, the action will run Packer at the latest version to init, validate, and build the example configuration
within the [example](example) folder. This is useful to quickly test a basic template of your plugin against Packer.

The example must contain the `required_plugins` block and require your plugin at the latest or any other released version.
This will help test and validate plugin releases.

## Registering Documentation on Packer.io

Documentation for a plugin is maintained within the `docs` directory and served on GitHub.
To include plugin docs on Packer.io a global pre-hook has been added to the main scaffolding .goreleaser.yml file, that if uncommented will generate and include a docs.zip file as part of the plugin release.

The `docs.zip` file will contain all of the `.mdx` files under the plugins root `docs/` directory that can be consumed remotely by Packer.io.

Once the first `docs.zip` file has been included into a release you will need to open a one time pull-request against [hashicorp/packer](https://github.com/hashicorp/packer) to register the plugin docs.
This is done by adding the block below for the respective plugin to the file [website/data/docs-remote-navigation.js](https://github.com/hashicorp/packer/blob/master/website/data/docs-remote-plugins.json).

```json
{
   "title": "Scaffolding",
   "path": "scaffolding",
   "repo": "hashicorp/packer-plugin-scaffolding",
   "version": "latest",
   "sourceBranch": "main"
 }
```

If a plugin maintainer wishes to only include a specific version of released docs then the `"version"` key in the above configuration should be set to a released version of the plugin. Otherwise it should be set to `"latest"`.

The `"sourceBranch"` key in the above configuration ensures potential contributors can link back to source files in the plugin repository from the Packer docs site. If a `"sourceBranch"` value is not present, it will default to `"main"`. 

The documentation structure needed for Packer.io can be generated manually, by creating a simple zip file called `docs.zip` of the docs directory and included in the plugin release.

```/bin/bash
[[ -d docs/ ]] && zip -r docs.zip docs/
```

Once the first `docs.zip` file has been included into a release you will need to open a one time pull-request against [hashicorp/packer](https://github.com/hashicorp/packer) to register the plugin docs.

# Requirements

-	[packer-plugin-sdk](https://github.com/hashicorp/packer-plugin-sdk) >= v0.1.0
-	[Go](https://golang.org/doc/install) >= 1.16

## Packer Compatibility
This scaffolding template is compatible with Packer >= v1.7.0
