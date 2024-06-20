Type: `proxmox-clone`
Artifact BuilderId: `proxmox.clone`

The `proxmox-clone` Packer builder is able to create new images for use with
[Proxmox](https://www.proxmox.com/en/proxmox-ve). The builder takes a virtual
machine template, runs any provisioning necessary on the image after launching it,
then creates a virtual machine template. This template can then be used as to
create new virtual machines within Proxmox.

Disks specified in a `proxmox-clone` builder configuration will replace disks
that are already present on the cloned VM template. If you want to reuse the disks
of the cloned VM, don't specify disks in your configuration.

The builder does _not_ manage templates. Once it creates a template, it is up
to you to use it or delete it.

## Configuration Reference

<!-- Code generated from the comments of the Config struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

There are many configuration options available for the builder. They are
segmented below into two categories: required and optional parameters. Within
each category, the available configuration keys are alphabetized.

You may also want to take look at the general configuration references for
[VirtIO RNG device](#virtio-rng-device)
and [PCI Devices](#pci-devices)
configuration references, which can be found further down the page.

In addition to the options listed here, a
[communicator](/packer/docs/templates/legacy_json_templates/communicator) can be configured for this
builder.

If no communicator is defined, an SSH key is generated for use, and is used
in the image's Cloud-Init settings for provisioning.

<!-- End of code generated from the comments of the Config struct in builder/proxmox/common/config.go; -->


### Required:

<!-- Code generated from the comments of the Config struct in builder/proxmox/clone/config.go; DO NOT EDIT MANUALLY -->

- `clone_vm` (string) - The name of the VM Packer should clone and build from.
  Either `clone_vm` or `clone_vm_id` must be specifed.

- `clone_vm_id` (int) - The ID of the VM Packer should clone and build from.
  Proxmox VMIDs are limited to the range 100-999999999.
  Either `clone_vm` or `clone_vm_id` must be specifed.

<!-- End of code generated from the comments of the Config struct in builder/proxmox/clone/config.go; -->


<!-- Code generated from the comments of the CDConfig struct in multistep/commonsteps/extra_iso_config.go; DO NOT EDIT MANUALLY -->

An iso (CD) containing custom files can be made available for your build.

By default, no extra CD will be attached. All files listed in this setting
get placed into the root directory of the CD and the CD is attached as the
second CD device.

This config exists to work around modern operating systems that have no
way to mount floppy disks, which was our previous go-to for adding files at
boot time.

<!-- End of code generated from the comments of the CDConfig struct in multistep/commonsteps/extra_iso_config.go; -->


<!-- Code generated from the comments of the CDConfig struct in multistep/commonsteps/extra_iso_config.go; DO NOT EDIT MANUALLY -->

- `cd_files` ([]string) - A list of files to place onto a CD that is attached when the VM is
  booted. This can include either files or directories; any directories
  will be copied onto the CD recursively, preserving directory structure
  hierarchy. Symlinks will have the link's target copied into the directory
  tree on the CD where the symlink was. File globbing is allowed.
  
  Usage example (JSON):
  
  ```json
  "cd_files": ["./somedirectory/meta-data", "./somedirectory/user-data"],
  "cd_label": "cidata",
  ```
  
  Usage example (HCL):
  
  ```hcl
  cd_files = ["./somedirectory/meta-data", "./somedirectory/user-data"]
  cd_label = "cidata"
  ```
  
  The above will create a CD with two files, user-data and meta-data in the
  CD root. This specific example is how you would create a CD that can be
  used for an Ubuntu 20.04 autoinstall.
  
  Since globbing is also supported,
  
  ```hcl
  cd_files = ["./somedirectory/*"]
  cd_label = "cidata"
  ```
  
  Would also be an acceptable way to define the above cd. The difference
  between providing the directory with or without the glob is whether the
  directory itself or its contents will be at the CD root.
  
  Use of this option assumes that you have a command line tool installed
  that can handle the iso creation. Packer will use one of the following
  tools:
  
    * xorriso
    * mkisofs
    * hdiutil (normally found in macOS)
    * oscdimg (normally found in Windows as part of the Windows ADK)

- `cd_content` (map[string]string) - Key/Values to add to the CD. The keys represent the paths, and the values
  contents. It can be used alongside `cd_files`, which is useful to add large
  files without loading them into memory. If any paths are specified by both,
  the contents in `cd_content` will take precedence.
  
  Usage example (HCL):
  
  ```hcl
  cd_files = ["vendor-data"]
  cd_content = {
    "meta-data" = jsonencode(local.instance_data)
    "user-data" = templatefile("user-data", { packages = ["nginx"] })
  }
  cd_label = "cidata"
  ```

- `cd_label` (string) - CD Label

<!-- End of code generated from the comments of the CDConfig struct in multistep/commonsteps/extra_iso_config.go; -->


### Optional:

<!-- Code generated from the comments of the Config struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

- `boot_key_interval` (duration string | ex: "1h5m2s") - Boot Key Interval

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

- `node` (string) - Which node in the Proxmox cluster to start the virtual
  machine on during creation.

- `pool` (string) - Name of resource pool to create virtual machine in.

- `task_timeout` (duration string | ex: "1h5m2s") - `task_timeout` (duration string | ex: "10m") - The timeout for
   Promox API operations, e.g. clones. Defaults to 1 minute.

- `vm_name` (string) - Name of the virtual machine during creation. If not
  given, a random uuid will be used.

- `vm_id` (int) - `vm_id` (int) - The ID used to reference the virtual machine. This will
  also be the ID of the final template. Proxmox VMIDs are unique cluster-wide
  and are limited to the range 100-999999999.
  If not given, the next free ID on the cluster will be used.

- `tags` (string) - The tags to set. This is a semicolon separated list. For example,
  `debian-12;template`.

- `boot` (string) - Override default boot order. Format example `order=virtio0;ide2;net0`.
  Prior to Proxmox 6.2-15 the format was `cdn` (c:CDROM -> d:Disk -> n:Network)

- `memory` (int) - How much memory (in megabytes) to give the virtual
  machine. If `ballooning_minimum` is also set, `memory` defines the maximum amount
  of memory the VM will be able to use.
  Defaults to `512`.

- `ballooning_minimum` (int) - Setting this option enables KVM memory ballooning and
  defines the minimum amount of memory (in megabytes) the VM will have.
  Defaults to `0` (memory ballooning disabled).

- `cores` (int) - How many CPU cores to give the virtual machine. Defaults
  to `1`.

- `cpu_type` (string) - The CPU type to emulate. See the Proxmox API
  documentation for the complete list of accepted values. For best
  performance, set this to `host`. Defaults to `kvm64`.

- `sockets` (int) - How many CPU sockets to give the virtual machine.
  Defaults to `1`

- `numa` (bool) - If true, support for non-uniform memory access (NUMA)
  is enabled. Defaults to `false`.

- `os` (string) - The operating system. Can be `wxp`, `w2k`, `w2k3`, `w2k8`,
  `wvista`, `win7`, `win8`, `win10`, `l24` (Linux 2.4), `l26` (Linux 2.6+),
  `solaris` or `other`. Defaults to `other`.

- `bios` (string) - Set the machine bios. This can be set to ovmf or seabios. The default value is seabios.

- `efi_config` (efiConfig) - Set the efidisk storage options. See [EFI Config](#efi-config).

- `efidisk` (string) - This option is deprecated, please use `efi_config` instead.

- `machine` (string) - Set the machine type. Supported values are 'pc' or 'q35'.

- `rng0` (rng0Config) - Configure Random Number Generator via VirtIO. See [VirtIO RNG device](#virtio-rng-device)

- `tpm_config` (tpmConfig) - Set the tpmstate storage options. See [TPM Config](#tpm-config).

- `vga` (vgaConfig) - The graphics adapter to use. See [VGA Config](#vga-config).

- `network_adapters` ([]NICConfig) - The network adapter to use. See [Network Adapters](#network-adapters)

- `disks` ([]diskConfig) - Disks attached to the virtual machine. See [Disks](#disks)

- `pci_devices` ([]pciDeviceConfig) - Allows passing through a host PCI device into the VM. See [PCI Devices](#pci-devices)

- `serials` ([]string) - A list (max 4 elements) of serial ports attached to
  the virtual machine. It may pass through a host serial device `/dev/ttyS0`
  or create unix socket on the host `socket`. Each element can be `socket`
  or responding to pattern `/dev/.+`. Example:
  
    ```json
    [
      "socket",
      "/dev/ttyS1"
    ]
    ```

- `qemu_agent` (boolean) - Enables QEMU Agent option for this VM. When enabled,
  then `qemu-guest-agent` must be installed on the guest. When disabled, then
  `ssh_host` should be used. Defaults to `true`.

- `scsi_controller` (string) - The SCSI controller model to emulate. Can be `lsi`,
  `lsi53c810`, `virtio-scsi-pci`, `virtio-scsi-single`, `megasas`, or `pvscsi`.
  Defaults to `lsi`.

- `onboot` (bool) - Specifies whether a VM will be started during system
  bootup. Defaults to `false`.

- `disable_kvm` (bool) - Disables KVM hardware virtualization. Defaults to `false`.

- `template_name` (string) - Name of the template. Defaults to the generated
  name used during creation.

- `template_description` (string) - Description of the template, visible in
  the Proxmox interface.

- `cloud_init` (bool) - If true, add an empty Cloud-Init CDROM drive after the virtual
  machine has been converted to a template. Defaults to `false`.

- `cloud_init_storage_pool` (string) - Name of the Proxmox storage pool
  to store the Cloud-Init CDROM on. If not given, the storage pool of the boot device will be used.

- `cloud_init_disk_type` (string) - The type of Cloud-Init disk. Can be `scsi`, `sata`, or `ide`
  Defaults to `ide`.

- `additional_iso_files` ([]ISOsConfig) - ISO files attached to the virtual machine.
  See [ISOs](#isos).

- `vm_interface` (string) - Name of the network interface that Packer gets
  the VMs IP from. Defaults to the first non loopback interface.

- `qemu_additional_args` (string) - Arbitrary arguments passed to KVM.
  For example `-no-reboot -smbios type=0,vendor=FOO`.
  	Note: this option is for experts only.

<!-- End of code generated from the comments of the Config struct in builder/proxmox/common/config.go; -->


<!-- Code generated from the comments of the Config struct in builder/proxmox/clone/config.go; DO NOT EDIT MANUALLY -->

- `full_clone` (boolean) - Whether to run a full or shallow clone from the base clone_vm. Defaults to `true`.

- `nameserver` (string) - Set nameserver IP address(es) via Cloud-Init.
  If not given, the same setting as on the host is used.

- `searchdomain` (string) - Set the DNS searchdomain via Cloud-Init.
  If not given, the same setting as on the host is used.

- `ipconfig` ([]cloudInitIpconfig) - Set IP address and gateway via Cloud-Init.
  See the [CloudInit Ip Configuration](#cloudinit-ip-configuration) documentation for fields.

<!-- End of code generated from the comments of the Config struct in builder/proxmox/clone/config.go; -->


### VGA Config

<!-- Code generated from the comments of the vgaConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

- `vga` (object) - The graphics adapter to use. Example:

	```json
	{
	  "type": "vmware",
	  "memory": 32
	}
	```

<!-- End of code generated from the comments of the vgaConfig struct in builder/proxmox/common/config.go; -->


#### Optional:

<!-- Code generated from the comments of the vgaConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

- `type` (string) - Can be `cirrus`, `none`, `qxl`,`qxl2`, `qxl3`,
  `qxl4`, `serial0`, `serial1`, `serial2`, `serial3`, `std`, `virtio`, `vmware`.
  Defaults to `std`.

- `memory` (int) - How much memory to assign.

<!-- End of code generated from the comments of the vgaConfig struct in builder/proxmox/common/config.go; -->


### Network Adapters

<!-- Code generated from the comments of the NICConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

Network adapters attached to the virtual machine.

Example:

```json
[

	{
	  "model": "virtio",
	  "bridge": "vmbr0",
	  "vlan_tag": "10",
	  "firewall": true
	}

]
```

<!-- End of code generated from the comments of the NICConfig struct in builder/proxmox/common/config.go; -->


#### Optional:

<!-- Code generated from the comments of the NICConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

- `model` (string) - Model of the virtual network adapter. Can be
  `rtl8139`, `ne2k_pci`, `e1000`, `pcnet`, `virtio`, `ne2k_isa`,
  `i82551`, `i82557b`, `i82559er`, `vmxnet3`, `e1000-82540em`,
  `e1000-82544gc` or `e1000-82545em`. Defaults to `e1000`.

- `packet_queues` (int) - Number of packet queues to be used on the device.
  Values greater than 1 indicate that the multiqueue feature is activated.
  For best performance, set this to the number of cores available to the
  virtual machine. CPU load on the host and guest systems will increase as
  the traffic increases, so activate this option only when the VM has to
  handle a great number of incoming connections, such as when the VM is
  operating as a router, reverse proxy or a busy HTTP server. Requires
  `virtio` network adapter. Defaults to `0`.

- `mac_address` (string) - Give the adapter a specific MAC address. If
  not set, defaults to a random MAC. If value is "repeatable", value of MAC
  address is deterministic based on VM ID and NIC ID.

- `mtu` (int) - Set the maximum transmission unit for the adapter. Valid
  range: 0 - 65520. If set to `1`, the MTU is inherited from the bridge
  the adapter is attached to. Defaults to `0` (use Proxmox default).

- `bridge` (string) - Required. Which Proxmox bridge to attach the
  adapter to.

- `vlan_tag` (string) - If the adapter should tag packets. Defaults to
  no tagging.

- `firewall` (bool) - If the interface should be protected by the firewall.
  Defaults to `false`.

<!-- End of code generated from the comments of the NICConfig struct in builder/proxmox/common/config.go; -->


### Disks

<!-- Code generated from the comments of the diskConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

Disks attached to the virtual machine.

Example:

```json
[

	{
	  "type": "scsi",
	  "disk_size": "5G",
	  "storage_pool": "local-lvm",
	  "storage_pool_type": "lvm"
	}

]
```

<!-- End of code generated from the comments of the diskConfig struct in builder/proxmox/common/config.go; -->


#### Optional:

<!-- Code generated from the comments of the diskConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

- `type` (string) - The type of disk. Can be `scsi`, `sata`, `virtio` or
  `ide`. Defaults to `scsi`.

- `storage_pool` (string) - Required. Name of the Proxmox storage pool
  to store the virtual machine disk on. A `local-lvm` pool is allocated
  by the installer, for example.

- `storage_pool_type` (string) - This option is deprecated.

- `disk_size` (string) - The size of the disk, including a unit suffix, such
  as `10G` to indicate 10 gigabytes.

- `cache_mode` (string) - How to cache operations to the disk. Can be
  `none`, `writethrough`, `writeback`, `unsafe` or `directsync`.
  Defaults to `none`.

- `format` (string) - The format of the file backing the disk. Can be
  `raw`, `cow`, `qcow`, `qed`, `qcow2`, `vmdk` or `cloop`. Defaults to
  `raw`.

- `io_thread` (bool) - Create one I/O thread per storage controller, rather
  than a single thread for all I/O. This can increase performance when
  multiple disks are used. Requires `virtio-scsi-single` controller and a
  `scsi` or `virtio` disk. Defaults to `false`.

- `asyncio` (string) - Configure Asynchronous I/O. Can be `native`, `threads`, or `io_uring`.
  Defaults to io_uring.

- `discard` (bool) - Relay TRIM commands to the underlying storage. Defaults
  to false. See the
  [Proxmox documentation](https://pve.proxmox.com/pve-docs/pve-admin-guide.html#qm_hard_disk_discard)
  for for further information.

- `ssd` (bool) - Drive will be presented to the guest as solid-state drive
  rather than a rotational disk.
  
  This cannot work with virtio disks.

<!-- End of code generated from the comments of the diskConfig struct in builder/proxmox/common/config.go; -->


### CloudInit Ip Configuration

<!-- Code generated from the comments of the cloudInitIpconfig struct in builder/proxmox/clone/config.go; DO NOT EDIT MANUALLY -->

If you have configured more than one network interface, make sure to match the order of
`network_adapters` and `ipconfig`.

Usage example (JSON):

```json
[

	{
	  "ip": "192.168.1.55/24",
	  "gateway": "192.168.1.1",
	  "ip6": "fda8:a260:6eda:20::4da/128",
	  "gateway6": "fda8:a260:6eda:20::1"
	}

]
```

<!-- End of code generated from the comments of the cloudInitIpconfig struct in builder/proxmox/clone/config.go; -->


<!-- Code generated from the comments of the cloudInitIpconfig struct in builder/proxmox/clone/config.go; DO NOT EDIT MANUALLY -->

- `ip` (string) - Either an IPv4 address (CIDR notation) or `dhcp`.

- `gateway` (string) - IPv4 gateway.

- `ip6` (string) - Can be an IPv6 address (CIDR notation), `auto` (enables SLAAC), or `dhcp`.

- `gateway6` (string) - IPv6 gateway.

<!-- End of code generated from the comments of the cloudInitIpconfig struct in builder/proxmox/clone/config.go; -->


### ISO Files

<!-- Code generated from the comments of the ISOsConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

ISO files attached to the virtual machine.

JSON Example:

```json

	"additional_iso_files": [
		{
			  "type": "scsi",
			  "iso_file": "local:iso/virtio-win-0.1.185.iso",
			  "unmount": true,
			  "iso_checksum": "af2b3cc9fa7905dea5e58d31508d75bba717c2b0d5553962658a47aebc9cc386"
		}
	 ]

```
HCL2 example:

```hcl

	additional_iso_files {
	  type = "scsi"
	  iso_file = "local:iso/virtio-win-0.1.185.iso"
	  unmount = true
	  iso_checksum = "af2b3cc9fa7905dea5e58d31508d75bba717c2b0d5553962658a47aebc9cc386"
	}

```

<!-- End of code generated from the comments of the ISOsConfig struct in builder/proxmox/common/config.go; -->


<!-- Code generated from the comments of the ISOConfig struct in multistep/commonsteps/iso_config.go; DO NOT EDIT MANUALLY -->

By default, Packer will symlink, download or copy image files to the Packer
cache into a "`hash($iso_url+$iso_checksum).$iso_target_extension`" file.
Packer uses [hashicorp/go-getter](https://github.com/hashicorp/go-getter) in
file mode in order to perform a download.

go-getter supports the following protocols:

* Local files
* Git
* Mercurial
* HTTP
* Amazon S3

Examples:
go-getter can guess the checksum type based on `iso_checksum` length, and it is
also possible to specify the checksum type.

In JSON:

```json

	"iso_checksum": "946a6077af6f5f95a51f82fdc44051c7aa19f9cfc5f737954845a6050543d7c2",
	"iso_url": "ubuntu.org/.../ubuntu-14.04.1-server-amd64.iso"

```

```json

	"iso_checksum": "file:ubuntu.org/..../ubuntu-14.04.1-server-amd64.iso.sum",
	"iso_url": "ubuntu.org/.../ubuntu-14.04.1-server-amd64.iso"

```

```json

	"iso_checksum": "file://./shasums.txt",
	"iso_url": "ubuntu.org/.../ubuntu-14.04.1-server-amd64.iso"

```

```json

	"iso_checksum": "file:./shasums.txt",
	"iso_url": "ubuntu.org/.../ubuntu-14.04.1-server-amd64.iso"

```

In HCL2:

```hcl

	iso_checksum = "946a6077af6f5f95a51f82fdc44051c7aa19f9cfc5f737954845a6050543d7c2"
	iso_url = "ubuntu.org/.../ubuntu-14.04.1-server-amd64.iso"

```

```hcl

	iso_checksum = "file:ubuntu.org/..../ubuntu-14.04.1-server-amd64.iso.sum"
	iso_url = "ubuntu.org/.../ubuntu-14.04.1-server-amd64.iso"

```

```hcl

	iso_checksum = "file://./shasums.txt"
	iso_url = "ubuntu.org/.../ubuntu-14.04.1-server-amd64.iso"

```

```hcl

	iso_checksum = "file:./shasums.txt",
	iso_url = "ubuntu.org/.../ubuntu-14.04.1-server-amd64.iso"

```

<!-- End of code generated from the comments of the ISOConfig struct in multistep/commonsteps/iso_config.go; -->


#### Required

<!-- Code generated from the comments of the ISOConfig struct in multistep/commonsteps/iso_config.go; DO NOT EDIT MANUALLY -->

- `iso_checksum` (string) - The checksum for the ISO file or virtual hard drive file. The type of
  the checksum is specified within the checksum field as a prefix, ex:
  "md5:{$checksum}". The type of the checksum can also be omitted and
  Packer will try to infer it based on string length. Valid values are
  "none", "{$checksum}", "md5:{$checksum}", "sha1:{$checksum}",
  "sha256:{$checksum}", "sha512:{$checksum}" or "file:{$path}". Here is a
  list of valid checksum values:
   * md5:090992ba9fd140077b0661cb75f7ce13
   * 090992ba9fd140077b0661cb75f7ce13
   * sha1:ebfb681885ddf1234c18094a45bbeafd91467911
   * ebfb681885ddf1234c18094a45bbeafd91467911
   * sha256:ed363350696a726b7932db864dda019bd2017365c9e299627830f06954643f93
   * ed363350696a726b7932db864dda019bd2017365c9e299627830f06954643f93
   * file:http://releases.ubuntu.com/20.04/SHA256SUMS
   * file:file://./local/path/file.sum
   * file:./local/path/file.sum
   * none
  Although the checksum will not be verified when it is set to "none",
  this is not recommended since these files can be very large and
  corruption does happen from time to time.

- `iso_url` (string) - A URL to the ISO containing the installation image or virtual hard drive
  (VHD or VHDX) file to clone.

<!-- End of code generated from the comments of the ISOConfig struct in multistep/commonsteps/iso_config.go; -->


#### Optional

<!-- Code generated from the comments of the ISOConfig struct in multistep/commonsteps/iso_config.go; DO NOT EDIT MANUALLY -->

- `iso_urls` ([]string) - Multiple URLs for the ISO to download. Packer will try these in order.
  If anything goes wrong attempting to download or while downloading a
  single URL, it will move on to the next. All URLs must point to the same
  file (same checksum). By default this is empty and `iso_url` is used.
  Only one of `iso_url` or `iso_urls` can be specified.

- `iso_target_path` (string) - The path where the iso should be saved after download. By default will
  go in the packer cache, with a hash of the original filename and
  checksum as its name.

- `iso_target_extension` (string) - The extension of the iso file after download. This defaults to `iso`.

<!-- End of code generated from the comments of the ISOConfig struct in multistep/commonsteps/iso_config.go; -->


<!-- Code generated from the comments of the ISOsConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

- `type` (string) - Bus type and bus index that the ISO will be mounted on. Can be `ide`,
  `sata` or `scsi`.
  Defaults to `ide`.

- `iso_file` (string) - Path to the ISO file to boot from, expressed as a
  proxmox datastore path, for example
  `local:iso/Fedora-Server-dvd-x86_64-29-1.2.iso`.
  Either `iso_file` OR `iso_url` must be specifed.

- `iso_storage_pool` (string) - Proxmox storage pool onto which to upload
  the ISO file.

- `iso_download_pve` (bool) - Download the ISO directly from the PVE node rather than through Packer.
  
  Defaults to `false`

- `unmount` (bool) - If true, remove the mounted ISO from the template after finishing. Defaults to `false`.

- `keep_cdrom_device` (bool) - Keep CDRom device attached to template if unmounting ISO. Defaults to `false`.
  Has no effect if unmount is `false`

<!-- End of code generated from the comments of the ISOsConfig struct in builder/proxmox/common/config.go; -->


<!-- Code generated from the comments of the CDConfig struct in multistep/commonsteps/extra_iso_config.go; DO NOT EDIT MANUALLY -->

An iso (CD) containing custom files can be made available for your build.

By default, no extra CD will be attached. All files listed in this setting
get placed into the root directory of the CD and the CD is attached as the
second CD device.

This config exists to work around modern operating systems that have no
way to mount floppy disks, which was our previous go-to for adding files at
boot time.

<!-- End of code generated from the comments of the CDConfig struct in multistep/commonsteps/extra_iso_config.go; -->


<!-- Code generated from the comments of the CDConfig struct in multistep/commonsteps/extra_iso_config.go; DO NOT EDIT MANUALLY -->

- `cd_files` ([]string) - A list of files to place onto a CD that is attached when the VM is
  booted. This can include either files or directories; any directories
  will be copied onto the CD recursively, preserving directory structure
  hierarchy. Symlinks will have the link's target copied into the directory
  tree on the CD where the symlink was. File globbing is allowed.
  
  Usage example (JSON):
  
  ```json
  "cd_files": ["./somedirectory/meta-data", "./somedirectory/user-data"],
  "cd_label": "cidata",
  ```
  
  Usage example (HCL):
  
  ```hcl
  cd_files = ["./somedirectory/meta-data", "./somedirectory/user-data"]
  cd_label = "cidata"
  ```
  
  The above will create a CD with two files, user-data and meta-data in the
  CD root. This specific example is how you would create a CD that can be
  used for an Ubuntu 20.04 autoinstall.
  
  Since globbing is also supported,
  
  ```hcl
  cd_files = ["./somedirectory/*"]
  cd_label = "cidata"
  ```
  
  Would also be an acceptable way to define the above cd. The difference
  between providing the directory with or without the glob is whether the
  directory itself or its contents will be at the CD root.
  
  Use of this option assumes that you have a command line tool installed
  that can handle the iso creation. Packer will use one of the following
  tools:
  
    * xorriso
    * mkisofs
    * hdiutil (normally found in macOS)
    * oscdimg (normally found in Windows as part of the Windows ADK)

- `cd_content` (map[string]string) - Key/Values to add to the CD. The keys represent the paths, and the values
  contents. It can be used alongside `cd_files`, which is useful to add large
  files without loading them into memory. If any paths are specified by both,
  the contents in `cd_content` will take precedence.
  
  Usage example (HCL):
  
  ```hcl
  cd_files = ["vendor-data"]
  cd_content = {
    "meta-data" = jsonencode(local.instance_data)
    "user-data" = templatefile("user-data", { packages = ["nginx"] })
  }
  cd_label = "cidata"
  ```

- `cd_label` (string) - CD Label

<!-- End of code generated from the comments of the CDConfig struct in multistep/commonsteps/extra_iso_config.go; -->


### EFI Config

<!-- Code generated from the comments of the efiConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

Set the efidisk storage options.
This needs to be set if you use ovmf uefi boot (supersedes the `efidisk` option).

Usage example (JSON):

```json

	{
	  "efi_storage_pool": "local",
	  "pre_enrolled_keys": true,
	  "efi_format": "raw",
	  "efi_type": "4m"
	}

```

<!-- End of code generated from the comments of the efiConfig struct in builder/proxmox/common/config.go; -->


#### Optional:

<!-- Code generated from the comments of the efiConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

- `efi_storage_pool` (string) - Name of the Proxmox storage pool to store the EFI disk on.

- `efi_format` (string) - The format of the file backing the disk. Can be
  `raw`, `cow`, `qcow`, `qed`, `qcow2`, `vmdk` or `cloop`. Defaults to
  `raw`.

- `pre_enrolled_keys` (bool) - Whether Microsoft Standard Secure Boot keys should be pre-loaded on
  the EFI disk. Defaults to `false`.

- `efi_type` (string) - Specifies the version of the OVMF firmware to be used. Can be `2m` or `4m`.
  Defaults to `4m`.

<!-- End of code generated from the comments of the efiConfig struct in builder/proxmox/common/config.go; -->


### VirtIO RNG device

<!-- Code generated from the comments of the rng0Config struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

- `rng0` (object): Configure Random Number Generator via VirtIO.
A virtual hardware-RNG can be used to provide entropy from the host system to a guest VM helping avoid entropy starvation which might cause the guest system slow down.
The device is sourced from a host device and guest, his use can be limited: `max_bytes` bytes of data will become available on a `period` ms timer.
[PVE documentation](https://pve.proxmox.com/pve-docs/pve-admin-guide.html) recommends to always use a limiter to avoid guests using too many host resources.

HCL2 example:

```hcl

	rng0 {
	  source    = "/dev/urandom"
	  max_bytes = 1024
	  period    = 1000
	}

```

JSON example:

```json

	{
	    "rng0": {
	        "source": "/dev/urandom",
	        "max_bytes": 1024,
	        "period": 1000
	    }
	}

```

<!-- End of code generated from the comments of the rng0Config struct in builder/proxmox/common/config.go; -->


#### Required:

<!-- Code generated from the comments of the rng0Config struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

- `source` (string) - Device on the host to gather entropy from.
  `/dev/urandom` should be preferred over `/dev/random` as Proxmox PVE documentation suggests.
  `/dev/hwrng` can be used to pass through a hardware RNG.
  Can be one of `/dev/urandom`, `/dev/random`, `/dev/hwrng`.

- `max_bytes` (int) - Maximum bytes of entropy allowed to get injected into the guest every `period` milliseconds.
  Use a lower value when using `/dev/random` since can lead to entropy starvation on the host system.
  `0` disables limiting and according to PVE documentation is potentially dangerous for the host.
  Recommended value: `1024`.

<!-- End of code generated from the comments of the rng0Config struct in builder/proxmox/common/config.go; -->


#### Optional:

<!-- Code generated from the comments of the rng0Config struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

- `period` (int) - Period in milliseconds on which the the entropy-injection quota is reset.
  Can be a positive value.
  Recommended value: `1000`.

<!-- End of code generated from the comments of the rng0Config struct in builder/proxmox/common/config.go; -->


### PCI devices

<!-- Code generated from the comments of the pciDeviceConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

Allows passing through a host PCI device into the VM. For example, a graphics card
or a network adapter. Devices that are mapped into a guest VM are no longer available
on the host. A minimal configuration only requires either the `host` or the `mapping`
key to be specifed.

Note: VMs with passed-through devices cannot be migrated.

HCL2 example:

```hcl

	pci_devices {
	  host          = "0000:0d:00.1"
	  pcie          = false
	  device_id     = "1003"
	  legacy_igd    = false
	  mdev          = "some-model"
	  hide_rombar   = false
	  romfile       = "vbios.bin"
	  sub_device_id = ""
	  sub_vendor_id = ""
	  vendor_id     = "15B3"
	  x_vga         = false
	}

```

JSON example:

```json

	{
	  "pci_devices": {
	    "host"          : "0000:0d:00.1",
	    "pcie"          : false,
	    "device_id"     : "1003",
	    "legacy_igd"    : false,
	    "mdev"          : "some-model",
	    "hide_rombar"   : false,
	    "romfile"       : "vbios.bin",
	    "sub_device_id" : "",
	    "sub_vendor_id" : "",
	    "vendor_id"     : "15B3",
	    "x_vga"         : false
	  }
	}

```

<!-- End of code generated from the comments of the pciDeviceConfig struct in builder/proxmox/common/config.go; -->


#### Optional:

<!-- Code generated from the comments of the pciDeviceConfig struct in builder/proxmox/common/config.go; DO NOT EDIT MANUALLY -->

- `host` (string) - The PCI ID of a host’s PCI device or a PCI virtual function. You can us the `lspci` command to list existing PCI devices. Either this or the `mapping` key must be set.

- `device_id` (string) - Override PCI device ID visible to guest.

- `legacy_igd` (bool) - Pass this device in legacy IGD mode, making it the primary and exclusive graphics device in the VM. Requires `pc-i440fx` machine type and VGA set to `none`. Defaults to `false`.

- `mapping` (string) - The ID of a cluster wide mapping. Either this or the `host` key must be set.

- `pcie` (bool) - Present the device as a PCIe device (needs `q35` machine model). Defaults to `false`.

- `mdev` (string) - The type of mediated device to use. An instance of this type will be created on startup of the VM and will be cleaned up when the VM stops.

- `hide_rombar` (bool) - Specify whether or not the device’s ROM BAR will be visible in the guest’s memory map. Defaults to `false`.

- `romfile` (string) - Custom PCI device rom filename (must be located in `/usr/share/kvm/`).

- `sub_device_id` (string) - Override PCI subsystem device ID visible to guest.

- `sub_vendor_id` (string) - Override PCI subsystem vendor ID visible to guest.

- `vendor_id` (string) - Override PCI vendor ID visible to guest.

- `x_vga` (bool) - Enable vfio-vga device support. Defaults to `false`.

<!-- End of code generated from the comments of the pciDeviceConfig struct in builder/proxmox/common/config.go; -->


## Example: Cloud-Init enabled Debian

Here is a basic example creating a Debian 10 server image. This assumes
that there exists a Cloud-Init enabled image on the Proxmox server named
`debian-10-4`.

**HCL2**

```hcl
variable "proxmox_password" {
  type    = string
  default = "supersecret"
}

variable "proxmox_username" {
  type    = string
  default = "apiuser@pve"
}

source "proxmox-clone" "debian" {
  clone_vm                 = "debian-10-4"
  cores                    = 1
  insecure_skip_tls_verify = true
  memory                   = 2048
  network_adapters {
    bridge = "vmbr0"
    model  = "virtio"
  }
  node                 = "pve"
  os                   = "l26"
  password             = "${var.proxmox_password}"
  pool                 = "api-users"
  proxmox_url          = "https://my-proxmox.my-domain:8006/api2/json"
  sockets              = 1
  ssh_username         = "root"
  template_description = "image made from cloud-init image"
  template_name        = "debian-scaffolding"
  username             = "${var.proxmox_username}"
}

build {
  sources = ["source.proxmox-clone.debian"]
}
```

**JSON**

```json
{
  "variables": {
    "proxmox_username": "apiuser@pve",
    "proxmox_password": "supersecret"
  },
  "builders": [
    {
      "type": "proxmox-clone",
      "proxmox_url": "https://my-proxmox.my-domain:8006/api2/json",
      "username": "{{user `proxmox_username`}}",
      "password": "{{user `proxmox_password`}}",
      "ssh_username": "root",
      "node": "pve",
      "insecure_skip_tls_verify": true,
      "clone_vm": "debian-10-4",
      "template_name": "debian-scaffolding",
      "template_description": "image made from cloud-init image",
      "pool": "api-users",
      "os": "l26",
      "cores": 1,
      "sockets": 1,
      "memory": 2048,
      "network_adapters": [
        {
          "model": "virtio",
          "bridge": "vmbr0"
        }
      ]
    }
  ]
}
```
