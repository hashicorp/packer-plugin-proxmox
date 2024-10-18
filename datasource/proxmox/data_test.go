package proxmoxtemplate

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const mockListResponse = `{"data":[
	{"name":"first-vm","cpu":0,"maxdisk":34359738368,"maxmem":2147483648,"diskread":512,"diskwrite":0,"netout":0,"disk":0,"status":"running","template":0,"type":"qemu","uptime":3,"node":"pve","maxcpu":1,"netin":52,"vmid":100,"mem":36052190,"id":"qemu/100"},
	{"id":"qemu/101","vmid":101,"netin":0,"mem":0,"type":"qemu","maxcpu":1,"uptime":0,"node":"pve","tags":"blue;red","diskread":0,"diskwrite":0,"netout":0,"status":"stopped","disk":0,"template":0,"maxmem":2147483648,"maxdisk":34359738368,"cpu":0,"name":"second-vm"}
]}`

func TestExecute(t *testing.T) {
	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/cluster/resources" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, mockListResponse)
		}
	}))
	defer mockAPI.Close()

	pxmxURL, _ := url.Parse(mockAPI.URL)
	config := Config{
		proxmoxURL:         pxmxURL,
		SkipCertValidation: true,
		Username:           "dummy@vmhost",
		Token:              "dummy",
	}

	ds := Datasource{
		config: config,
	}

	result, err := ds.Execute()
	require.NoError(t, err)
	t.Log(result)

	//client, err := newProxmoxClient(config)
	//require.NoError(t, err)
	//
	//vmList, err := proxmox.ListGuests(client)
	//require.NoError(t, err)
	//require.Equal(t, uint(100), vmList[0].Id)
	//require.Equal(t, []proxmox.Tag{"blue", "red"}, vmList[1].Tags)
	//t.Log(vmList)

}
