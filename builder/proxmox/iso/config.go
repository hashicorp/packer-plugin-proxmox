//go:generate packer-sdc mapstructure-to-hcl2 -type Config,nicConfig,diskConfig,vgaConfig,additionalISOsConfig

package proxmoxiso

import (
	"context"
	"encoding/hex"
	"errors"
	"path"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/go-getter/v2"
	proxmoxcommon "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type Config struct {
	proxmoxcommon.Config `mapstructure:",squash"`

	commonsteps.ISOConfig `mapstructure:",squash"`
	ISOFile               string `mapstructure:"iso_file"`
	ISOStoragePool        string `mapstructure:"iso_storage_pool"`
	ISODownloadPVE        bool   `mapstructure:"iso_download_pve"`
	UnmountISO            bool   `mapstructure:"unmount_iso"`
	shouldUploadISO       bool
}

func (c *Config) Prepare(raws ...interface{}) ([]string, []string, error) {
	var errs *packersdk.MultiError
	_, warnings, merrs := c.Config.Prepare(c, raws...)
	if merrs != nil {
		errs = packersdk.MultiErrorAppend(errs, merrs)
	}

	// Check ISO config
	// Either a pre-uploaded ISO should be referenced in iso_file, OR a URL
	// (possibly to a local file) to an ISO file that will be downloaded and
	// then uploaded to Proxmox.
	// If iso_download_pve is true, iso_url will be downloaded directly to the
	// PVE node.
	if c.ISOFile != "" {
		c.shouldUploadISO = false
	} else {
		isoWarnings, isoErrors := c.ISOConfig.Prepare(&c.Ctx)
		errs = packersdk.MultiErrorAppend(errs, isoErrors...)
		warnings = append(warnings, isoWarnings...)
		c.shouldUploadISO = true
	}

	if (c.ISOFile == "" && len(c.ISOConfig.ISOUrls) == 0) || (c.ISOFile != "" && len(c.ISOConfig.ISOUrls) != 0) {
		errs = packersdk.MultiErrorAppend(errs, errors.New("either iso_file or iso_url, but not both, must be specified"))
	}
	if len(c.ISOConfig.ISOUrls) != 0 && c.ISOStoragePool == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("when specifying iso_url, iso_storage_pool must also be specified"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return nil, warnings, errs
	}
	return nil, warnings, nil
}

// Take ISOConfig configuration attributes in the format defined for packer-plugin-sdk
// and use go-getter to generate parameters compatible with the Proxmox-API.
func (c *Config) generateIsoConfigs() ([]proxmox.ConfigContent_Iso, error) {
	var isoConfigs []proxmox.ConfigContent_Iso
	var errs *packersdk.MultiError
	for _, url := range c.ISOUrls {
		var checksum string
		var checksumType string
		if c.ISOChecksum == "none" {
			checksum = ""
			checksumType = ""
		} else {
			gr := &getter.Request{
				Src: url + "?checksum=" + c.ISOChecksum,
			}
			gc := getter.Client{}
			fileChecksum, err := gc.GetChecksum(context.TODO(), gr)
			if err != nil {
				errs = packersdk.MultiErrorAppend(errs, err)
				continue
			}
			checksum = hex.EncodeToString(fileChecksum.Value)
			checksumType = fileChecksum.Type
		}
		isoConfigs = append(isoConfigs, proxmox.ConfigContent_Iso{
			Node:              c.Node,
			Storage:           c.ISOStoragePool,
			DownloadUrl:       url,
			Filename:          path.Base(url),
			ChecksumAlgorithm: checksumType,
			Checksum:          checksum,
		})
	}
	if errs != nil && len(errs.Errors) > 0 {
		return isoConfigs, errs
	}
	return isoConfigs, nil
}
