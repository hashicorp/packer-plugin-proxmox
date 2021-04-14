//go:generate mapstructure-to-hcl2 -type Config,DatasourceOutput
package scaffolding

import (
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/hcl2helper"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/zclconf/go-cty/cty"
)

type Config struct {
	MockOption string `mapstructure:"mock"`
}

type Datasource struct {
	config Config
}

type DatasourceOutput struct {
	Foo string `mapstructure:"foo"`
	Bar string `mapstructure:"bar"`
}

func (d *Datasource) ConfigSpec() hcldec.ObjectSpec {
	return d.config.FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Configure(raws ...interface{}) error {
	err := config.Decode(&d.config, nil, raws...)
	if err != nil {
		return err
	}
	return nil
}

func (d *Datasource) OutputSpec() hcldec.ObjectSpec {
	return (&DatasourceOutput{}).FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Execute() (cty.Value, error) {
	output := DatasourceOutput{
		Foo: "foo-value",
		Bar: "bar-value",
	}
	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}
