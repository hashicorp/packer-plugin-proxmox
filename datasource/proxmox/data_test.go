package proxmoxtemplate

import (
	"fmt"
	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const mockListResponse = `{"data":[{"diskread":0,"netout":0,"type":"qemu","name":"first-vm","vmid":100,"netin":0,"id":"qemu/100","maxmem":2147483648,"diskwrite":0,"maxcpu":1,"template":0,"status":"stopped","cpu":0,"mem":0,"uptime":0,"node":"pve","disk":0,"maxdisk":34359738368},{"netin":0,"maxmem":2147483648,"id":"qemu/101","diskwrite":0,"status":"stopped","maxcpu":1,"template":0,"diskread":0,"type":"qemu","netout":0,"name":"second-vm","tags":"blue;red","vmid":101,"node":"pve","disk":0,"maxdisk":34359738368,"cpu":0,"uptime":0,"mem":0},{"maxdisk":34359738368,"disk":0,"node":"pve","mem":0,"uptime":0,"cpu":0,"maxcpu":1,"template":1,"status":"stopped","diskwrite":0,"maxmem":2147483648,"id":"qemu/102","netin":0,"vmid":102,"tags":"blue","type":"qemu","netout":0,"name":"template-three","diskread":0}]}`
const mockConfig100Response = `{"data":{"net0":"virtio=BC:24:11:30:AD:93,bridge=vmbr0,firewall=1","ostype":"l26","digest":"1ac0a42d3ead982a5631ee77a9444b44e5ff5456","meta":"creation-qemu=8.1.5,ctime=1729285344","cpu":"x86-64-v2-AES","numa":0,"ide2":"none,media=cdrom","scsihw":"virtio-scsi-single","cores":1,"sockets":1,"boot":"order=scsi0;ide2;net0","vmgenid":"fe9dbfef-eb32-4c75-9e36-aaefc3cbda82","smbios1":"uuid=413c6ca3-d043-43fe-9fb4-bc4a6e60c7ef","memory":"2048","name":"first-vm","scsi0":"local-lvm:vm-100-disk-0,iothread=1,size=32G"}}`
const mockConfig101Response = `{"data":{"digest":"a3a5061cd0f35f0aa14fd98b20095963d53cd225","net0":"virtio=BC:24:11:FA:B6:AB,bridge=vmbr0,firewall=1","ostype":"l26","numa":0,"cpu":"x86-64-v2-AES","meta":"creation-qemu=8.1.5,ctime=1729285359","boot":"order=scsi0;ide2;net0","sockets":1,"cores":1,"scsihw":"virtio-scsi-single","ide2":"none,media=cdrom","scsi0":"local-lvm:vm-101-disk-0,iothread=1,size=32G","name":"second-vm","memory":"2048","vmgenid":"48935acd-21f9-4733-a695-5072949deab5","tags":"blue;red","smbios1":"uuid=271b5345-f952-4c99-a299-b66d71b4e736"}}`
const mockConfig102Response = `{"data":{"memory":"2048","smbios1":"uuid=95ef26a6-78e7-46f9-9728-9a222213f198","ide2":"none,media=cdrom","template":1,"cores":1,"meta":"creation-qemu=8.1.5,ctime=1729285377","cpu":"x86-64-v2-AES","digest":"03ef3384bc6f151328671273f32d0df85ebe257f","net0":"virtio=BC:24:11:A4:A0:3E,bridge=vmbr0,firewall=1","ostype":"l26","tags":"blue","vmgenid":"364e8932-f1ad-4cb2-add8-c720a891c654","name":"template-three","scsi0":"local-lvm:base-102-disk-0,iothread=1,size=32G","sockets":1,"scsihw":"virtio-scsi-single","boot":"order=scsi0;ide2;net0","numa":0}}`

func TestExecute(t *testing.T) {
	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			switch path := r.URL.Path; path {
			case "/cluster/resources":
				fmt.Fprintln(w, mockListResponse)
			case "/nodes/pve/qemu/100/config":
				fmt.Fprintln(w, mockConfig100Response)
			case "/nodes/pve/qemu/101/config":
				fmt.Fprintln(w, mockConfig101Response)
			case "/nodes/pve/qemu/102/config":
				fmt.Fprintln(w, mockConfig102Response)
			default:
				return
			}
		}
	}))
	defer mockAPI.Close()

	pxmxURL, _ := url.Parse(mockAPI.URL)
	config := Config{
		proxmoxURL:         pxmxURL,
		SkipCertValidation: true,
		Username:           "dummy@vmhost",
		Token:              "dummy",
		Name:               "second-vm",
		//Latest:             true,
	}

	ds := Datasource{
		config: config,
	}

	result, err := ds.Execute()
	require.NoError(t, err)
	t.Log(result)
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
