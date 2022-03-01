package proxmox

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type uploaderMock struct {
	uploadFail      bool
	deleteFail      bool
	uploadWasCalled bool
	deleteWasCalled bool
}

func (m *uploaderMock) Upload(node string, storage string, contentType string, filename string, file io.Reader) error {
	m.uploadWasCalled = true
	if m.uploadFail {
		return fmt.Errorf("Testing induced Upload failure")
	}
	return nil
}

func (m *uploaderMock) DeleteVolume(vmr *proxmox.VmRef, storageName string, volumeName string) (exitStatus interface{}, err error) {
	m.deleteWasCalled = true
	if m.deleteFail {
		return nil, fmt.Errorf("Testing induced DeleteVolume failure")
	}
	return
}

var _ uploader = &uploaderMock{}

func TestUploadAdditionalISO(t *testing.T) {
	cs := []struct {
		name               string
		builderConfig      *Config
		step               *stepUploadAdditionalISO
		testAssert         func(m *uploaderMock, action multistep.StepAction)
		downloadPath       string
		generatedISOPath   string
		failUpload         bool
		failDelete         bool
		expectError        bool
		expectUploadCalled bool
		expectDeleteCalled bool
		expectedISOPath    string
		expectedAction     multistep.StepAction
	}{
		{
			name:          "should not call upload unless configured to do so",
			builderConfig: &Config{},
			step: &stepUploadAdditionalISO{
				ISO: &additionalISOsConfig{
					ShouldUploadISO: false,
				},
			},
			expectError:    false,
			expectedAction: multistep.ActionContinue,
		},
		{
			name:          "StepCreateCD not called (no cd_path present) should halt",
			builderConfig: &Config{},
			step: &stepUploadAdditionalISO{
				ISO: &additionalISOsConfig{
					ShouldUploadISO: true,
					CDConfig: commonsteps.CDConfig{
						CDFiles: []string{"testfile"},
					},
				},
			},
			expectError:    true,
			expectedAction: multistep.ActionHalt,
		},
		{
			name:          "DownloadPathKey not valid should halt",
			builderConfig: &Config{},
			step: &stepUploadAdditionalISO{
				ISO: &additionalISOsConfig{
					ShouldUploadISO: true,
					DownloadPathKey: "",
				},
			},
			expectError:    true,
			expectedAction: multistep.ActionHalt,
		},
		{
			name:          "ISO not found should halt",
			builderConfig: &Config{},
			step: &stepUploadAdditionalISO{
				ISO: &additionalISOsConfig{
					ShouldUploadISO: true,
					DownloadPathKey: "filethatdoesnotexist.iso",
				},
			},
			downloadPath:   "filethatdoesnotexist.iso",
			expectError:    true,
			expectedAction: multistep.ActionHalt,
		},
		{
			name:          "generated ISO should be uploaded and deleted",
			builderConfig: &Config{},
			step: &stepUploadAdditionalISO{
				ISO: &additionalISOsConfig{
					ShouldUploadISO: true,
					ISOStoragePool:  "local",
					CDConfig: commonsteps.CDConfig{
						CDFiles: []string{"testfile"},
					},
					DownloadPathKey: "../iso/testdata/test.iso",
				},
			},
			generatedISOPath: "../iso/testdata/test.iso",

			expectError:        false,
			expectedAction:     multistep.ActionContinue,
			expectUploadCalled: true,
			expectedISOPath:    "local:iso/test.iso",
			expectDeleteCalled: true,
		},
		{
			name:          "generated ISO should be uploaded but deletion failed",
			builderConfig: &Config{},
			step: &stepUploadAdditionalISO{
				ISO: &additionalISOsConfig{
					ShouldUploadISO: true,
					ISOStoragePool:  "local",
					CDConfig: commonsteps.CDConfig{
						CDFiles: []string{"testfile"},
					},
					DownloadPathKey: "../iso/testdata/test.iso",
				},
			},
			generatedISOPath:   "../iso/testdata/test.iso",
			failDelete:         true,
			expectError:        true,
			expectedAction:     multistep.ActionContinue,
			expectUploadCalled: true,
			expectedISOPath:    "local:iso/test.iso",
			expectDeleteCalled: true,
		},
		{
			name:          "downloaded ISO should be uploaded",
			builderConfig: &Config{},
			step: &stepUploadAdditionalISO{
				ISO: &additionalISOsConfig{
					ShouldUploadISO: true,
					ISOStoragePool:  "local",
					DownloadPathKey: "../iso/testdata/test.iso",
				},
			},
			downloadPath: "../iso/testdata/test.iso",

			expectError:        false,
			expectedAction:     multistep.ActionContinue,
			expectUploadCalled: true,
			expectedISOPath:    "local:iso/test.iso",
			expectDeleteCalled: false,
		},
		{
			name:          "downloaded ISO fail upload",
			builderConfig: &Config{},
			step: &stepUploadAdditionalISO{
				ISO: &additionalISOsConfig{
					ShouldUploadISO: true,
					ISOStoragePool:  "local",
					DownloadPathKey: "../iso/testdata/test.iso",
				},
			},
			downloadPath:       "../iso/testdata/test.iso",
			failUpload:         true,
			expectError:        true,
			expectedAction:     multistep.ActionHalt,
			expectUploadCalled: true,
			expectDeleteCalled: false,
		},
	}

	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			m := &uploaderMock{uploadFail: c.failUpload, deleteFail: c.failDelete}

			state := new(multistep.BasicStateBag)
			state.Put("ui", packersdk.TestUi(t))
			state.Put("config", c.builderConfig)
			state.Put("proxmoxClient", m)
			state.Put(c.step.ISO.DownloadPathKey, c.downloadPath)
			state.Put("cd_path", c.generatedISOPath)

			step := c.step
			action := step.Run(context.TODO(), state)
			step.Cleanup(state)

			if action != c.expectedAction {
				t.Errorf("Expected action to be %v, got %v", c.expectedAction, action)
			}
			if m.uploadWasCalled != c.expectUploadCalled {
				t.Errorf("Expected mock upload to be called: %v, got: %v", c.expectUploadCalled, m.uploadWasCalled)
			}
			if m.deleteWasCalled != c.expectDeleteCalled {
				t.Errorf("Expected mock delete to be called: %v, got: %v", c.expectDeleteCalled, m.deleteWasCalled)
			}
			err, gotError := state.GetOk("error")
			if gotError != c.expectError {
				t.Errorf("Expected error state to be: %v, got: %v", c.expectError, gotError)
			}
			if err == nil {
				if c.step.ISO.ISOFile != c.expectedISOPath {
					t.Errorf("Expected state iso_path to be %q, got %q", c.expectedISOPath, c.step.ISO.ISOFile)
				}
			}
		})
	}
}
