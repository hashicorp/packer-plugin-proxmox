// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,DatasourceOutput

package virtualmachine

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/hcl2helper"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/zclconf/go-cty/cty"
)

// Datasource has a bunch of filters which you can use, for example, to find the latest available
// template in the cluster that matches defined filters.
//
// You can combine any number of filters but all of them will be conjuncted with AND.
// When datasource cannot return only one (zero or >1) guest identifiers it will return error.
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
	// Filter that returns `vm_id` for virtual machine which name exactly matches this value.
	// Options `name` and `name_regex` are mutually exclusive.
	Name string `mapstructure:"name"`
	// Filter that returns `vm_id` for virtual machine which name matches the regular expression.
	// Expression must use [Go Regex Syntax](https://pkg.go.dev/regexp/syntax).
	// Options `name` and `name_regex` are mutually exclusive.
	NameRegex string `mapstructure:"name_regex"`
	// Filter that returns virtual machine `vm_id` only when virtual machine is template.
	Template bool `mapstructure:"template"`
	// Filter that returns `vm_id` only when virtual machine is located on the specified PVE node.
	Node string `mapstructure:"node"`
	// Filter that returns `vm_id` for virtual machine which has all these tags. When you need to
	// specify more than one tag, use semicolon as separator (`"tag1;tag2"`).
	// Every specified tag must exist in virtual machine.
	VmTags string `mapstructure:"vm_tags"`
	// This filter determines how to handle multiple virtual machines that were matched with all
	// previous filters. Virtual machine creation time is being used to find latest.
	// By default, multiple matching virtual machines results in an error.
	Latest bool `mapstructure:"latest"`
}

type Datasource struct {
	config Config
}

type DatasourceOutput struct {
	// Identifier of the found virtual machine.
	VmId uint `mapstructure:"vm_id"`
	// Name of the found virtual machine.
	VmName string `mapstructure:"vm_name"`
	// Tags of the found virtual machine separated with semicolon.
	VmTags string `mapstructure:"vm_tags"`
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
	var vmName, vmTags string

	client, err := newProxmoxClient(d.config)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}

	vmList, err := proxmox.ListGuests(client)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}

	filteredVms := filterGuests(d.config, vmList)
	if len(filteredVms) == 0 {
		return cty.NullVal(cty.EmptyObject), errors.New("no virtual machine matches the filters")
	}

	if d.config.Latest {
		vmConfigList, err := getVmConfigs(client, filteredVms)
		if err != nil {
			return cty.NullVal(cty.EmptyObject), err
		}

		latestConfig, err := findLatestConfig(vmConfigList)
		if err != nil {
			return cty.NullVal(cty.EmptyObject), err
		}

		vmId = latestConfig["vmid"].(uint)
		vmName = configValueOrEmpty(&latestConfig, "name")
		vmTags = configValueOrEmpty(&latestConfig, "tags")
	} else {
		if len(filteredVms) > 1 {
			return cty.NullVal(cty.EmptyObject), errors.New("more than one virtual machine matched the filters")
		}
		vmId = filteredVms[0].Id
		vmName = filteredVms[0].Name
		vmTags = joinTags(filteredVms[0].Tags, ";")
	}

	output := DatasourceOutput{
		VmId:   vmId,
		VmName: vmName,
		VmTags: vmTags,
	}
	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}

// findLatestConfig finds the latest VM among those passed using `configs`.
// The `meta` field contains info about creation time (but it is not described in API docs).
func findLatestConfig(configs []vmConfig) (vmConfig, error) {
	var result vmConfig
	var maxCtime int
	for i := range configs {
		if metaField, ok := configs[i]["meta"]; ok {
			vmCtime, err := parseMetaField(metaField.(string))
			if err != nil {
				return nil, err
			}
			if vmCtime > maxCtime {
				maxCtime = vmCtime
				result = configs[i]
			}
		} else {
			return nil, errors.New("no meta field in the virtual machine config")
		}
	}
	return result, nil
}

// getVmConfigs retrieves configs from PVE in 'map[string]interface{}' format for all VMs in the list.
// Also add value of VM ID to every config (useful for further steps).
func getVmConfigs(client *proxmox.Client, vmList []proxmox.GuestResource) ([]vmConfig, error) {
	var result []vmConfig
	for _, vm := range vmList {
		var thisConfig vmConfig
		vmr := proxmox.NewVmRef(int(vm.Id))
		thisConfig, err := client.GetVmConfig(vmr)
		if err != nil {
			return nil, err
		}
		thisConfig["vmid"] = vm.Id
		result = append(result, thisConfig)
	}
	return result, nil
}

// filterGuests removes virtual machines from the `guests` list that do not match some filters in the datasource config.
func filterGuests(config Config, guests []proxmox.GuestResource) []proxmox.GuestResource {
	filterFuncs := make([]func(proxmox.GuestResource) bool, 0)

	if config.Name != "" {
		filterFuncs = append(filterFuncs, func(vm proxmox.GuestResource) bool {
			return vm.Name == config.Name
		})
	}

	if config.NameRegex != "" {
		filterFuncs = append(filterFuncs, func(vm proxmox.GuestResource) bool {
			return regexp.MustCompile(config.NameRegex).MatchString(vm.Name)
		})
	}

	if config.Template {
		filterFuncs = append(filterFuncs, func(vm proxmox.GuestResource) bool {
			return vm.Template
		})
	}

	if config.Node != "" {
		filterFuncs = append(filterFuncs, func(vm proxmox.GuestResource) bool {
			return vm.Node == config.Node
		})
	}

	if config.VmTags != "" {
		// Split tags string because it can contain several tags separated with ";"
		tagsSplitted := strings.Split(config.VmTags, ";")
		filterFuncs = append(filterFuncs, func(vm proxmox.GuestResource) bool {
			return len(vm.Tags) > 0 && configTagsMatchNodeTags(tagsSplitted, vm.Tags)
		})
	}

	result := make([]proxmox.GuestResource, 0)
	for _, guest := range guests {
		var ok bool
		if len(filterFuncs) == 0 {
			ok = true
		}
		for _, guestPassedFilter := range filterFuncs {
			if ok = guestPassedFilter(guest); !ok {
				break
			}
		}
		if ok {
			result = append(result, guest)
		}
	}

	return result
}

// configTagsMatchNodeTags compares two lists of strings and returns true only when all
// elements from the first list are present in the second list.
func configTagsMatchNodeTags(configTags []string, nodeTags []proxmox.Tag) bool {
	var countOfMatchedTags int
	for _, configTag := range configTags {
		var matched bool
		for _, nodeTag := range nodeTags {
			if configTag == string(nodeTag) {
				matched = true
				break
			}
		}
		if matched {
			countOfMatchedTags += 1
		}
	}
	if countOfMatchedTags != len(configTags) {
		return false
	}
	return true
}

// newProxmoxClient creates new client and tries to connect and log in to Proxmox instance.
func newProxmoxClient(config Config) (*proxmox.Client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.SkipCertValidation,
	}

	client, err := proxmox.NewClient(strings.TrimSuffix(config.proxmoxURL.String(), "/"), nil, "", tlsConfig, "", int(config.TaskTimeout.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("could not connect to Proxmox: %w", err)
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
			return nil, fmt.Errorf("could not log in to Proxmox: %w", err)
		}
	}

	return client, nil
}

// parseMetaField parses the string from the `meta` field and returns integer value
// representing the creation date of the virtual machine in epoch seconds format.
func parseMetaField(field string) (int, error) {
	re, err := regexp.Compile(`.*ctime=(?P<ctime>[0-9]+).*`)
	if err != nil {
		return 0, fmt.Errorf("could not compile regex to parse meta field: %w", err)
	}

	matched := re.MatchString(field)
	if !matched {
		return 0, nil
	}
	valueStr := re.ReplaceAllString(field, "${ctime}")
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, fmt.Errorf("could not convert date field to int: %w", err)
	}
	return value, nil
}

// joinTags used to combine list of strings into one string with defined separator.
func joinTags(tags []proxmox.Tag, separator string) string {
	tagsAsStrings := make([]string, len(tags))
	for i, tag := range tags {
		tagsAsStrings[i] = string(tag)
	}
	return strings.Join(tagsAsStrings, separator)
}

// configValueOrEmpty tries to retrieve string by key from dynamic map of interfaces.
// In case when key not found or there was an error, this function returns empty string.
func configValueOrEmpty(values *vmConfig, key string) string {
	result := ""
	if values != nil {
		value, exists := (*values)[key]
		if !exists {
			return result
		}
		strValue, ok := value.(string)
		if !ok {
			return result
		}
		result = strValue
	}
	return result
}
