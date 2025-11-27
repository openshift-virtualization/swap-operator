package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctrlcommon "github.com/openshift-virtualization/swap-operator/internal/common"
	"github.com/openshift-virtualization/swap-operator/internal/renderconfig"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
)

func TestGenerateMachineConfigForName(t *testing.T) {
	tests := []struct {
		name         string
		config       *renderconfig.RenderConfig
		role         string
		mcName       string
		setupFunc    func(t *testing.T, templateDir string)
		wantErr      bool
		validateFunc func(t *testing.T, mc *mcfgv1.MachineConfig)
	}{
		{
			name: "basic worker config with file-based swap",
			config: &renderconfig.RenderConfig{
				Name:                "99-filebased-swap-0",
				Index:               0,
				EnableFileBasedSwap: true,
				SwapFileSize:        "1Gi",
				SwapFilePath:        "/var/tmp/swapfile",
				SwapDevicePriotiry:  10,
			},
			role:   "worker",
			mcName: "99-filebased-swap-0",
			setupFunc: func(t *testing.T, templateDir string) {
				workerPath := filepath.Join(templateDir, "worker", "99-filebased-swap-0", "_base")
				filesPath := filepath.Join(workerPath, "files")
				unitsPath := filepath.Join(workerPath, "units")

				os.MkdirAll(filesPath, 0755)
				os.MkdirAll(unitsPath, 0755)

				fileTemplate := `path: /etc/swap.conf
mode: 0644
contents:
  inline: |
    swap enabled
`
				os.WriteFile(filepath.Join(filesPath, "swap.conf"), []byte(fileTemplate), 0644)

				unitTemplate := `{{- if .EnableFileBasedSwap }}
name: filbased-swap-provision-{{ .Index }}.service
enabled: true
contents: |
  [Unit]
  Description=Provision and enable swap
  ConditionFirstBoot=no
  ConditionPathExists=!{{ .SwapFilePath }}
  
  [Service]
  Type=oneshot
  Environment=SWAP_SIZE={{ .SwapFileSize }}
  ExecStart=/bin/sh -c "sudo fallocate -l ${SWAP_SIZE} {{ .SwapFilePath }} && \
  sudo chmod 600 {{ .SwapFilePath }} && \
  sudo mkswap {{ .SwapFilePath }} && \
  sudo swapon {{ .SwapFilePath }}"
  
  [Install]
  RequiredBy=kubelet-dependencies.target
{{- end}}
`
				os.WriteFile(filepath.Join(unitsPath, "swap.service"), []byte(unitTemplate), 0644)
			},
			wantErr: false,
			validateFunc: func(t *testing.T, mc *mcfgv1.MachineConfig) {
				if mc == nil {
					t.Fatal("expected non-nil MachineConfig")
				}

				ignCfg, err := ctrlcommon.ParseAndConvertConfig(mc.Spec.Config.Raw)
				if err != nil {
					t.Fatalf("failed to parse ignition config: %v", err)
				}

				// Check units with rendered template
				if len(ignCfg.Systemd.Units) != 1 {
					t.Errorf("expected 1 unit, got %d", len(ignCfg.Systemd.Units))
				} else {
					unit := ignCfg.Systemd.Units[0]
					if unit.Name != "filbased-swap-provision-0.service" {
						t.Errorf("wrong unit name: %s", unit.Name)
					}

					if unit.Enabled == nil || *unit.Enabled != true {
						t.Error("missing expected enabled for unit")
					}

					if unit.Contents != nil && !strings.Contains(*unit.Contents, "/var/tmp/swapfile") {
						t.Errorf("template SwapFilePath rendered incorrectly, got %s", *unit.Contents)
					}

					if unit.Contents != nil && !strings.Contains(*unit.Contents, "SWAP_SIZE=1Gi") {
						t.Errorf("template SwapFileSize rendered incorrectly, got %s", *unit.Contents)
					}
				}
			},
		},
		{
			name:   "worker mandatory templates",
			config: &renderconfig.RenderConfig{},
			role:   "worker",
			mcName: "99-swap-kubelet-cgroups",
			setupFunc: func(t *testing.T, templateDir string) {
				commonPath := filepath.Join(templateDir, "worker", "99-swap-kubelet-cgroups", "_base", "files")
				os.MkdirAll(commonPath, 0755)
				commonTmpl := `mode: 0420
overwrite: true
path: "/etc/openshift/kubelet.conf.d/90-swap.conf"
contents:
  inline: |
    apiVersion: kubelet.config.k8s.io/v1beta1
    kind: KubeletConfiguration
    failSwapOn: false
    memorySwap:
      swapBehavior: LimitedSwap
`
				os.WriteFile(filepath.Join(commonPath, "kubeletdropin.yaml"), []byte(commonTmpl), 0644)

				workerPath := filepath.Join(templateDir, "worker", "99-swap-kubelet-cgroups", "_base", "units")
				os.MkdirAll(workerPath, 0755)
				workerTmpl := `name: system-slice-swap-disable.service
enabled: true
contents: |
  [Unit]
  Description=Restrict swap for system slice
  ConditionFirstBoot=no

  [Service]
  Type=oneshot
  ExecStart=/bin/sh -c "sudo systemctl set-property --runtime system.slice MemorySwapMax=0 IODeviceLatencyTargetSec=\"/ 50ms\""

  [Install]
  RequiredBy=kubelet-dependencies.target
`
				os.WriteFile(filepath.Join(workerPath, "system-slice-swap-disable.service"), []byte(workerTmpl), 0644)
			},
			validateFunc: func(t *testing.T, mc *mcfgv1.MachineConfig) {
				ignCfg, err := ctrlcommon.ParseAndConvertConfig(mc.Spec.Config.Raw)
				if err != nil {
					t.Fatalf("parse error: %v", err)
				}

				if len(ignCfg.Storage.Files) != 1 {
					t.Errorf("expected 1 file, got %d", len(ignCfg.Storage.Files))
				}

				if len(ignCfg.Systemd.Units) != 1 {
					t.Errorf("expected 1 unit, got %d", len(ignCfg.Systemd.Units))
				}

				file := ignCfg.Storage.Files[0]
				if file.Path != "/etc/openshift/kubelet.conf.d/90-swap.conf" {
					t.Error("missing expected file path for kubelet drop-in")
				}
				if file.Overwrite == nil || *file.Overwrite != true {
					t.Error("missing expected overwrite for kubelet drop-in")
				}
				if file.Mode == nil || *file.Mode != 0420 {
					t.Error("missing expected mode for kubelet drop-in")
				}

				unit := ignCfg.Systemd.Units[0]
				if unit.Name != "system-slice-swap-disable.service" {
					t.Error("missing expected unit name")
				}

				if unit.Enabled == nil || *unit.Enabled != true {
					t.Error("missing expected enabled for unit")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, _ := os.MkdirTemp("", "template-test-*")
			defer os.RemoveAll(tempDir)

			if tt.setupFunc != nil {
				tt.setupFunc(t, tempDir)
			}

			path := filepath.Join(tempDir, tt.role, tt.mcName)
			got, err := GenerateMachineConfigForName(tt.config, tt.role, tt.mcName, tempDir, path)

			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, got)
			}
		})
	}
}
