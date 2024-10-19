// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc mapstructure-to-hcl2 -type Config,DatasourceOutput
package proxmoxtemplate

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/hcl2helper"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/zclconf/go-cty/cty"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	common.PackerConfig `mapstructure:",squash"`

	// URL to the Proxmox API, including the full path,
	// so `https://<server>:<port>/api2/json` for example.
	// Can also be set via the `PROXMOX_URL` environment variable.
	ProxmoxURLRaw string `mapstructure:"proxmox_url"`
	proxmoxURL    *url.URL
	// Skip validating the certificate.
	SkipCertValidation bool `mapstructure:"insecure_skip_tls_verify"`
	// Username when authenticating to Proxmox, including
	// the realm. For example `user@pve` to use the local Proxmox realm. When using
	// token authentication, the username must include the token id after an exclamation
	// mark. For example, `user@pve!tokenid`.
	// Can also be set via the `PROXMOX_USERNAME` environment variable.
	Username string `mapstructure:"username"`
	// Password for the user.
	// For API tokens please use `token`.
	// Can also be set via the `PROXMOX_PASSWORD` environment variable.
	// Either `password` or `token` must be specifed. If both are set,
	// `token` takes precedence.
	Password string `mapstructure:"password"`
	// Token for authenticating API calls.
	// This allows the API client to work with API tokens instead of user passwords.
	// Can also be set via the `PROXMOX_TOKEN` environment variable.
	// Either `password` or `token` must be specifed. If both are set,
	// `token` takes precedence.
	Token string `mapstructure:"token"`
	// `task_timeout` (duration string | ex: "10m") - The timeout for
	//  Promox API operations, e.g. clones. Defaults to 1 minute.
	TaskTimeout time.Duration `mapstructure:"task_timeout"`
	// Exact name of the guest to return. Options `name` and `name_regex` are mutually exclusive.
	Name string `mapstructure:"name"`
	// Regex matching the name of the guest to return. Options `name` and `name_regex` are mutually exclusive.
	NameRegex string `mapstructure:"name_regex"`
	// Boolean to return only guest of template type.
	Template bool `mapstructure:"template"`
	// Get VMID for the latest created guest. This is useful when defined filters
	// return more than one guest (by default multiple guests result in error).
	Latest bool `mapstructure:"latest"`
}

type Datasource struct {
	config Config
}

type DatasourceOutput struct {
	VmId uint `mapstructure:"vm_id"`
}

type vmConfig map[string]interface{}

func (d *Datasource) ConfigSpec() hcldec.ObjectSpec {
	return d.config.FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Configure(raws ...interface{}) error {
	err := config.Decode(&d.config, nil, raws...)
	if err != nil {
		return err
	}

	var errs *packersdk.MultiError

	// Defaults
	if d.config.ProxmoxURLRaw == "" {
		d.config.ProxmoxURLRaw = os.Getenv("PROXMOX_URL")
	}
	if d.config.Username == "" {
		d.config.Username = os.Getenv("PROXMOX_USERNAME")
	}
	if d.config.Password == "" {
		d.config.Password = os.Getenv("PROXMOX_PASSWORD")
	}
	if d.config.Token == "" {
		d.config.Token = os.Getenv("PROXMOX_TOKEN")
	}

	// Required configurations that will display errors if not set
	if d.config.Username == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("username must be specified"))
	}
	if d.config.Password == "" && d.config.Token == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("password or token must be specified"))
	}
	if d.config.ProxmoxURLRaw == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("proxmox_url must be specified"))
	}
	if d.config.proxmoxURL, err = url.Parse(d.config.ProxmoxURLRaw); err != nil {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse proxmox_url: %s", err))
	}

	if d.config.Name == "" && d.config.NameRegex == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("name or name_regex must be specified"))
	}

	if d.config.Name != "" && d.config.NameRegex != "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("name and name_regex are mutually exclusive"))
	}

	if d.config.NameRegex != "" {
		if _, err := regexp.Compile(d.config.NameRegex); err != nil {
			errs = packersdk.MultiErrorAppend(errs, errors.New("cannot compile regex string"))
		}
	}

	if d.config.TaskTimeout == 0 {
		d.config.TaskTimeout = 60 * time.Second
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}
	return nil
}

func (d *Datasource) OutputSpec() hcldec.ObjectSpec {
	return (&DatasourceOutput{}).FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Execute() (cty.Value, error) {
	// This value of VM ID the function should return
	var vmId uint

	client, err := newProxmoxClient(d.config)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}

	vmList, err := proxmox.ListGuests(client)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}

	filteredVmIds := filterGuests(d.config, vmList)
	for i := range vmList {
		filteredVmIds[i] = vmList[i].Id
	}

	var vmConfigList []vmConfig
	if d.config.Latest {
		// Get configs from PVE in 'map[string]interface{}' format for all VMs in the list.
		for _, id := range filteredVmIds {
			var thisConfig vmConfig
			vmr := proxmox.NewVmRef(int(id))
			thisConfig, err = client.GetVmConfig(vmr)
			if err != nil {
				return cty.NullVal(cty.EmptyObject), err
			}
			thisConfig["vmid"] = id
			vmConfigList = append(vmConfigList, thisConfig)
		}

		// Find the latest VM among filtered.
		// The `meta` field contains info about creation time (but it is not described in API docs).
		var latestConfig vmConfig
		var maxCtime int
		for i := range vmConfigList {
			if metaField, ok := vmConfigList[i]["meta"]; ok {
				vmCtime, err := parseMetaField(metaField.(string))
				if err != nil {
					return cty.NullVal(cty.EmptyObject), err
				}
				if vmCtime > maxCtime {
					maxCtime = vmCtime
					latestConfig = vmConfigList[i]
				}
			} else {
				return cty.NullVal(cty.EmptyObject), errors.New("no meta field in the guest config")
			}
		}
		vmId = latestConfig["vmid"].(uint)
	} else {
		if len(filteredVmIds) > 1 {
			return cty.NullVal(cty.EmptyObject), errors.New("more than one guest passed filters, cannot return vm_id")
		}
		vmId = filteredVmIds[0]
	}

	output := DatasourceOutput{
		VmId: vmId,
	}
	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}

func filterGuests(config Config, guests []proxmox.GuestResource) []proxmox.GuestResource {
	result := make([]proxmox.GuestResource, 0)
	if config.Name != "" {
		result = filterByName(guests, config.Name)
	}
	if config.NameRegex != "" {
		result = filterByNameRegex(guests, config.NameRegex)
	}
	applicable := make([]proxmox.GuestResource, 0)
	if len(result) == 0 {
		applicable = guests
	} else {
		applicable = result
	}

	if config.Template {
		result = filterByTemplate(applicable)
	}

	return result
}

func filterByName(guests []proxmox.GuestResource, name string) []proxmox.GuestResource {
	result := make([]proxmox.GuestResource, 0)
	for _, i := range guests {
		if i.Name == name {
			result = append(result, i)
		}
	}
	return result
}

func filterByNameRegex(guests []proxmox.GuestResource, nameRegex string) []proxmox.GuestResource {
	re, _ := regexp.Compile(nameRegex)
	result := make([]proxmox.GuestResource, 0)
	for _, i := range guests {
		if re.MatchString(i.Name) {
			result = append(result, i)
		}
	}
	return result
}

func filterByTemplate(guests []proxmox.GuestResource) []proxmox.GuestResource {
	result := make([]proxmox.GuestResource, 0)
	for _, i := range guests {
		if i.Template {
			result = append(result, i)
		}
	}
	return result
}

func newProxmoxClient(config Config) (*proxmox.Client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.SkipCertValidation,
	}

	client, err := proxmox.NewClient(strings.TrimSuffix(config.proxmoxURL.String(), "/"), nil, "", tlsConfig, "", int(config.TaskTimeout.Seconds()))
	if err != nil {
		return nil, err
	}

	*proxmox.Debug = config.PackerDebug

	if config.Token != "" {
		// configure token auth
		log.Print("using token auth")
		client.SetAPIToken(config.Username, config.Token)
	} else {
		// fallback to login if not using tokens
		log.Print("using password auth")
		err = client.Login(config.Username, config.Password, "")
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}

func parseMetaField(field string) (int, error) {
	re, err := regexp.Compile(`.*ctime=(?P<ctime>[0-9]+).*`)
	if err != nil {
		return 0, err
	}

	matched := re.MatchString(field)
	if !matched {
		return 0, nil
	}
	valueStr := re.ReplaceAllString(field, "${ctime}")
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, err
	}
	return value, nil
}
