//go:generate packer-sdc mapstructure-to-hcl2 -type Config,cloudInitIpconfig

package proxmoxclone

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	proxmoxcommon "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

type Config struct {
	proxmoxcommon.Config `mapstructure:",squash"`

	CloneVM   string         `mapstructure:"clone_vm" required:"true"`
	FullClone config.Trilean `mapstructure:"full_clone" required:"false"`

	Nameserver   string              `mapstructure:"nameserver" required:"false"`
	Searchdomain string              `mapstructure:"searchdomain" required:"false"`
	Ipconfigs    []cloudInitIpconfig `mapstructure:"ipconfig" required:"false"`
}

type cloudInitIpconfig struct {
	Ip       string `mapstructure:"ip" required:"false"`
	Gateway  string `mapstructure:"gateway" required:"false"`
	Ip6      string `mapstructure:"ip6" required:"false"`
	Gateway6 string `mapstructure:"gateway6" required:"false"`
}

func (c *Config) Prepare(raws ...interface{}) ([]string, []string, error) {
	var errs *packersdk.MultiError
	_, warnings, merrs := c.Config.Prepare(c, raws...)
	if merrs != nil {
		errs = packersdk.MultiErrorAppend(errs, merrs)
	}

	if c.CloneVM == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("clone_vm must be specified"))
	}

	// Check validity of given IP addresses
	if c.Nameserver != "" {
		for _, nameserver := range strings.Split(c.Nameserver, " ") {
			_, err := netip.ParseAddr(nameserver)
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse nameserver: %s", err))
			}
		}
	}
	for _, i := range c.Ipconfigs {
		if i.Ip != "" {
			_, _, err := net.ParseCIDR(i.Ip)
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse ipconfig.ip: %s", err))
			}
		}
		if i.Gateway != "" {
			_, err := netip.ParseAddr(i.Gateway)
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse ipconfig.gateway: %s", err))
			}
		}
		if i.Ip6 != "" {
			_, _, err := net.ParseCIDR(i.Ip6)
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse ipconfig.ip6: %s", err))
			}
		}
		if i.Gateway6 != "" {
			_, err := netip.ParseAddr(i.Gateway6)
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse ipconfig.gateway6: %s", err))
			}
		}
	}
	if len(c.NICs) < len(c.Ipconfigs) {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("%d ipconfig blocks given, but only %d network interfaces defined", len(c.Ipconfigs), len(c.NICs)))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return nil, warnings, errs
	}
	return nil, warnings, nil
}

// Convert Ipconfig attributes into a Proxmox-API compatible string
func (c cloudInitIpconfig) String() string {
	options := []string{}
	if c.Ip != "" {
		options = append(options, "ip="+c.Ip)
	}
	if c.Gateway != "" {
		options = append(options, "gw="+c.Gateway)
	}
	if c.Ip6 != "" {
		options = append(options, "ip6="+c.Ip6)
	}
	if c.Gateway6 != "" {
		options = append(options, "gw6="+c.Gateway6)
	}
	return strings.Join(options, ",")
}
