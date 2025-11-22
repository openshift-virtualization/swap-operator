package common

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"text/template"

	fcctbase "github.com/coreos/fcct/base/v0_1"
	"github.com/coreos/go-semver/semver"
	ign3 "github.com/coreos/ignition/v2/config/v3_5"
	ign3types "github.com/coreos/ignition/v2/config/v3_5/types"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	"go.yaml.in/yaml/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TranspileCoreOSConfigToIgn transpiles Fedora CoreOS config to ignition
// internally it transpiles to Ign spec v3 config
func TranspileCoreOSConfigToIgn(files, units []string) (*ign3types.Config, error) {
	overwrite := true
	outConfig := ign3types.Config{}
	// Convert data to Ignition resources
	for _, contents := range files {
		f := new(fcctbase.File)
		if err := yaml.Unmarshal([]byte(contents), f); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %q into struct: %w", contents, err)
		}
		f.Overwrite = &overwrite

		// Add the file to the config
		var ctCfg fcctbase.Config
		ctCfg.Storage.Files = append(ctCfg.Storage.Files, *f)
		ign30Config, tSet, err := ctCfg.ToIgn3_0()
		if err != nil {
			return nil, fmt.Errorf("failed to transpile config to Ignition config %w\nTranslation set: %v", err, tSet)
		}
		ign3Config, err := ignitionConverter.Convert(ign30Config, *semver.New(ign30Config.Ignition.Version), ign3types.MaxVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to convert config from 3.0 to %v. %w", ign3types.MaxVersion, err)
		}
		outConfig = ign3.Merge(outConfig, ign3Config.(ign3types.Config))
	}

	for _, contents := range units {
		u := new(fcctbase.Unit)
		if err := yaml.Unmarshal([]byte(contents), u); err != nil {
			return nil, fmt.Errorf("failed to unmarshal systemd unit into struct: %w", err)
		}

		// Add the unit to the config
		var ctCfg fcctbase.Config
		ctCfg.Systemd.Units = append(ctCfg.Systemd.Units, *u)
		ign30Config, tSet, err := ctCfg.ToIgn3_0()
		if err != nil {
			return nil, fmt.Errorf("failed to transpile config to Ignition config %w\nTranslation set: %v", err, tSet)
		}
		ign3Config, err := ignitionConverter.Convert(ign30Config, *semver.New(ign30Config.Ignition.Version), ign3types.MaxVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to convert config from 3.0 to %v. %w", ign3types.MaxVersion, err)
		}
		outConfig = ign3.Merge(outConfig, ign3Config.(ign3types.Config))
	}

	return &outConfig, nil
}

// MachineConfigFromIgnConfig creates a MachineConfig with the provided Ignition config
func MachineConfigFromIgnConfig(role, name string, ignCfg interface{}) (*mcfgv1.MachineConfig, error) {
	rawIgnCfg, err := json.Marshal(ignCfg)
	if err != nil {
		return nil, fmt.Errorf("error marshalling Ignition config: %w", err)
	}
	return MachineConfigFromRawIgnConfig(role, name, rawIgnCfg)
}

// MachineConfigFromRawIgnConfig creates a MachineConfig with the provided raw Ignition config
func MachineConfigFromRawIgnConfig(role, name string, rawIgnCfg []byte) (*mcfgv1.MachineConfig, error) {
	labels := map[string]string{
		mcfgv1.MachineConfigRoleLabelKey: role,
	}
	return &mcfgv1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
			Name:   name,
		},
		Spec: mcfgv1.MachineConfigSpec{
			OSImageURL: "",
			Config: runtime.RawExtension{
				Raw: rawIgnCfg,
			},
		},
	}, nil
}

// ioutil.ReadDir has been deprecated with os.ReadDir.
// ioutil.ReadDir() used to return []fs.FileInfo but os.ReadDir() returns []fs.DirEntry.
// Making it helper function so that we can reuse coversion of []fs.DirEntry into []fs.FileInfo
// Implementation to fetch fileInfo is taken from https://pkg.go.dev/io/ioutil#ReadDir
func ReadDir(path string) ([]fs.FileInfo, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir %q: %w", path, err)
	}
	infos := make([]fs.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch fileInfo of %q in %q: %w", entry.Name(), path, err)
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// Configures common template FuncMaps used across all renderers.
func GetTemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"toString": strval,
		"indent":   indent,
	}
}

// Converts an interface to a string.
// Copied from: https://github.com/Masterminds/sprig/blob/master/strings.go
// Copied to remove the dependency on the Masterminds/sprig library.
func strval(v interface{}) string {
	switch v := v.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case error:
		return v.Error()
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Indents a string n spaces.
// Copied from: https://github.com/Masterminds/sprig/blob/master/strings.go
// Copied to remove the dependency on the Masterminds/sprig library.
func indent(spaces int, v string) string {
	pad := strings.Repeat(" ", spaces)
	return pad + strings.ReplaceAll(v, "\n", "\n"+pad)
}
