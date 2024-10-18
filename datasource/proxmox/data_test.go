package proxmoxtemplate

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/stretchr/testify/require"
)

// For the sake of saving code clean have left only test-related fields in these JSONs.
const mockResourcesResponse = `{"data":[
	{"type":"qemu","name":"first-vm","vmid":100,"id":"qemu/100","template":0,"node":"pve"},
	{"id":"qemu/101","template":0,"type":"qemu","name":"second-vm","tags":"blue;red","vmid":101,"node":"pve"},
	{"node":"pve","template":1,"id":"qemu/102","vmid":102,"tags":"blue","type":"qemu","name":"template-three"}
]}`
const mockVmConfig100Response = `{"data":
	{"meta":"creation-qemu=8.1.5,ctime=1729285344","name":"first-vm"}
}`
const mockVmConfig101Response = `{"data":
	{"meta":"creation-qemu=8.1.5,ctime=1729285359","name":"second-vm","tags":"blue;red"}
}`
const mockVmConfig102Response = `{"data":
	{"template":1,"meta":"creation-qemu=8.1.5,ctime=1729285377","tags":"blue","name":"template-three"}
}`

// All configs have to have all default fields so add them from `configDefault` (in place).
func (c *Config) saturateWithDefault(configDefault Config) {
	c.proxmoxURL = configDefault.proxmoxURL
	c.SkipCertValidation = configDefault.SkipCertValidation
	c.Username = configDefault.Username
	c.Token = configDefault.Token
}

func TestExecute(t *testing.T) {
	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			switch path := r.URL.Path; path {
			case "/cluster/resources":
				_, _ = fmt.Fprintln(w, mockResourcesResponse)
			case "/nodes/pve/qemu/100/config":
				_, _ = fmt.Fprintln(w, mockVmConfig100Response)
			case "/nodes/pve/qemu/101/config":
				_, _ = fmt.Fprintln(w, mockVmConfig101Response)
			case "/nodes/pve/qemu/102/config":
				_, _ = fmt.Fprintln(w, mockVmConfig102Response)
			default:
				return
			}
		}
	}))
	defer mockAPI.Close()

	pxmxURL, _ := url.Parse(mockAPI.URL)
	defaultConfig := Config{
		proxmoxURL:         pxmxURL,
		SkipCertValidation: true,
		Username:           "dummy@vmhost",
		Token:              "dummy",
	}

	dsTestConfigs := []struct {
		name          string
		expectFailure bool
		expectedVmId  int64
		configDiff    Config
	}{
		{
			name:          "guest with name first-vm found, no error",
			expectFailure: false,
			expectedVmId:  100,
			configDiff: Config{
				Name: "first-vm",
			},
		},
		{
			name:          "no existent guest matches filter, error",
			expectFailure: true,
			configDiff: Config{
				Name: "firstest-vm",
			},
		},
		{
			name:          "found guest by regex filter, no error",
			expectFailure: false,
			expectedVmId:  101,
			configDiff: Config{
				NameRegex: "sec.*",
			},
		},
		{
			name:          "multiple guests match the regex, but latest not used, error",
			expectFailure: true,
			configDiff: Config{
				NameRegex: ".*-vm",
			},
		},
		{
			name:          "multiple guests match the regex and latest used, error",
			expectFailure: false,
			expectedVmId:  101,
			configDiff: Config{
				NameRegex: ".*-vm",
				Latest:    true,
			},
		},
		{
			name:          "found guest that is template, no error",
			expectFailure: false,
			expectedVmId:  102,
			configDiff: Config{
				Template: true,
			},
		},
		{
			name:          "found latest guest at node, no error",
			expectFailure: false,
			expectedVmId:  102,
			configDiff: Config{
				Node:   "pve",
				Latest: true,
			},
		},
		{
			name:          "proxmox host not found, error",
			expectFailure: true,
			configDiff: Config{
				Node: "proxmox-host",
			},
		},
		{
			name:          "found guest with set of tags, no error",
			expectFailure: false,
			expectedVmId:  101,
			configDiff: Config{
				VmTags: "blue;red",
			},
		},
		{
			name:          "found multiple guests with tag, error",
			expectFailure: true,
			configDiff: Config{
				VmTags: "blue",
			},
		},
	}

	for _, dsTestConfig := range dsTestConfigs {
		t.Run(dsTestConfig.name, func(t *testing.T) {
			dsTestConfig.configDiff.saturateWithDefault(defaultConfig)
			ds := Datasource{
				config: dsTestConfig.configDiff,
			}

			result, err := ds.Execute()
			if err != nil && !dsTestConfig.expectFailure {
				t.Fatalf("unexpected failure: %s", err)
			}
			if err == nil && dsTestConfig.expectFailure {
				t.Errorf("expected failure, but execution succeeded")
			}
			if err == nil {
				vmIdInt64, _ := result.GetAttr("vm_id").AsBigFloat().Int64()
				vmName := result.GetAttr("vm_name").AsString()
				vmTags := result.GetAttr("vm_tags").AsString()
				t.Logf("Returned: vmId=%d, vmName=%s, vmTags=%s", vmIdInt64, vmName, vmTags)
				require.Equal(t, dsTestConfig.expectedVmId, vmIdInt64)
			}
		})
	}
}

func TestParseMetaField(t *testing.T) {
	const metaField = `creation-qemu=8.1.5,ctime=1729285377`
	result, err := parseMetaField(metaField)
	require.NoError(t, err)
	require.Equal(t, 1729285377, result)
}

func TestCompareTags(t *testing.T) {
	configTags := []string{"blue", "green"}
	nodeTags := []proxmox.Tag{"blue", "red"}
	require.Equal(t, false, configTagsMatchNodeTags(configTags, nodeTags))
}
