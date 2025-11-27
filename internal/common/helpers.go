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
	ign2error "github.com/coreos/ignition/config/shared/errors"
	ign2 "github.com/coreos/ignition/config/v2_2"
	ign2types "github.com/coreos/ignition/config/v2_2/types"
	ign3error "github.com/coreos/ignition/v2/config/shared/errors"
	ign3 "github.com/coreos/ignition/v2/config/v3_5"
	ign3types "github.com/coreos/ignition/v2/config/v3_5/types"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	"go.yaml.in/yaml/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
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

// Ensures SSH keys are unique for a given Ign 2 PasswdUser
// See: https://bugzilla.redhat.com/show_bug.cgi?id=1934176
func dedupePasswdUserSSHKeys(passwdUser ign2types.PasswdUser) ign2types.PasswdUser {
	// Map for checking for duplicates.
	knownSSHKeys := map[ign2types.SSHAuthorizedKey]bool{}

	// Preserve ordering of SSH keys.
	dedupedSSHKeys := []ign2types.SSHAuthorizedKey{}

	for _, sshKey := range passwdUser.SSHAuthorizedKeys {
		if _, isKnown := knownSSHKeys[sshKey]; isKnown {
			// We've seen this key before warn and move on.
			klog.Warningf("duplicate SSH public key found: %s", sshKey)
			continue
		}

		// We haven't seen this key before, add it.
		dedupedSSHKeys = append(dedupedSSHKeys, sshKey)
		knownSSHKeys[sshKey] = true
	}

	// Overwrite the keys with the deduped list.
	passwdUser.SSHAuthorizedKeys = dedupedSSHKeys

	return passwdUser
}

// Function to remove duplicated files/units/users from a V2 MC, since the translator
// (and ignition spec V3) does not allow for duplicated entries in one MC.
// This should really not change the actual final behaviour, since it keeps
// ordering into consideration and has contents from the highest alphanumeric
// MC's final version of a file.
// Note:
// Append is not considered since we do not allow for appending
// Units have one exception: dropins are concat'ed

func removeIgnDuplicateFilesUnitsUsers(ignConfig ign2types.Config) (ign2types.Config, error) {

	files := ignConfig.Storage.Files
	units := ignConfig.Systemd.Units
	users := ignConfig.Passwd.Users

	filePathMap := map[string]bool{}
	var outFiles []ign2types.File
	for i := len(files) - 1; i >= 0; i-- {
		// We do not actually support to other filesystems so we make the assumption that there is only 1 here
		path := files[i].Path
		if _, isDup := filePathMap[path]; isDup {
			continue
		}
		outFiles = append(outFiles, files[i])
		filePathMap[path] = true
	}

	unitNameMap := map[string]bool{}
	var outUnits []ign2types.Unit
	for i := len(units) - 1; i >= 0; i-- {
		unitName := units[i].Name
		if _, isDup := unitNameMap[unitName]; isDup {
			// this is a duplicated unit by name, so let's check for the dropins and append them
			if len(units[i].Dropins) > 0 {
				for j := range outUnits {
					if outUnits[j].Name == unitName {
						// outUnits[j] is the highest priority entry with this unit name
						// now loop over the new unit's dropins and append it if the name
						// isn't duplicated in the existing unit's dropins
						for _, newDropin := range units[i].Dropins {
							hasExistingDropin := false
							for _, existingDropins := range outUnits[j].Dropins {
								if existingDropins.Name == newDropin.Name {
									hasExistingDropin = true
									break
								}
							}
							if !hasExistingDropin {
								outUnits[j].Dropins = append(outUnits[j].Dropins, newDropin)
							}
						}
						continue
					}
				}
				klog.V(2).Infof("Found duplicate unit %v, appending dropin section", unitName)
			}
			continue
		}
		outUnits = append(outUnits, units[i])
		unitNameMap[unitName] = true
	}

	// Concat sshkey sections into the newest passwdUser in the list
	// We make the assumption that there is only one user: core
	// since that is the only supported user by design.
	// It's technically possible, though, to have created another user
	// during install time configs, since we only check the validity of
	// the passwd section if it was changed. Explicitly error in that case.
	if len(users) > 0 {
		outUser := users[len(users)-1]
		if outUser.Name != "core" {
			return ignConfig, fmt.Errorf("unexpected user with name: %v. Only core user is supported", outUser.Name)
		}
		for i := len(users) - 2; i >= 0; i-- {
			if users[i].Name != "core" {
				return ignConfig, fmt.Errorf("unexpected user with name: %v. Only core user is supported", users[i].Name)
			}
			for j := range users[i].SSHAuthorizedKeys {
				outUser.SSHAuthorizedKeys = append(outUser.SSHAuthorizedKeys, users[i].SSHAuthorizedKeys[j])
			}
		}
		// Ensure SSH key uniqueness
		ignConfig.Passwd.Users = []ign2types.PasswdUser{dedupePasswdUserSSHKeys(outUser)}
	}

	// outFiles and outUnits should now have all duplication removed
	ignConfig.Storage.Files = outFiles
	ignConfig.Systemd.Units = outUnits

	return ignConfig, nil
}

// IgnParseWrapper parses rawIgn for both V2 and V3 ignition configs and returns
// a V2 or V3 Config or an error. This wrapper is necessary since V2 and V3 use different parsers.
func IgnParseWrapper(rawIgn []byte) (interface{}, error) {
	// ParseCompatibleVersion will parse any config <= N to version N
	ignCfgV3, rptV3, errV3 := ign3.ParseCompatibleVersion(rawIgn)
	if errV3 == nil && !rptV3.IsFatal() {
		return ignCfgV3, nil
	}

	// ParseCompatibleVersion differentiates between ErrUnknownVersion ("I know what it is and we don't support it") and
	// ErrInvalidVersion ("I can't parse it to find out what it is"), but our old 3.2 logic didn't, so this is here to make sure
	// our error message for invalid version is still helpful.
	if errV3.Error() == ign3error.ErrInvalidVersion.Error() {
		versions := strings.TrimSuffix(strings.Join(IgnitionConverterSingleton().GetSupportedMinorVersions(), ","), ",")
		return ign3types.Config{}, fmt.Errorf("parsing Ignition config failed: invalid version. Supported spec versions: %s", versions)
	}

	if errV3.Error() == ign3error.ErrUnknownVersion.Error() {
		ignCfgV2, rptV2, errV2 := ign2.Parse(rawIgn)
		if errV2 == nil && !rptV2.IsFatal() {
			return ignCfgV2, nil
		}

		// If the error is still UnknownVersion it's not a 3.3/3.2/3.1/3.0 or 2.x config, thus unsupported
		if errV2.Error() == ign2error.ErrUnknownVersion.Error() {
			versions := strings.TrimSuffix(strings.Join(IgnitionConverterSingleton().GetSupportedMinorVersions(), ","), ",")
			return ign3types.Config{}, fmt.Errorf("parsing Ignition config failed: unknown version. Supported spec versions: %s", versions)
		}
		return ign3types.Config{}, fmt.Errorf("parsing Ignition spec v2 failed with error: %v\nReport: %v", errV2, rptV2)
	}

	return ign3types.Config{}, fmt.Errorf("parsing Ignition config spec v3 failed with error: %v\nReport: %v", errV3, rptV3)
}

// ParseAndConvertConfig parses rawIgn for both V2 and V3 ignition configs and returns
// a V3 or an error.
func ParseAndConvertConfig(rawIgn []byte) (ign3types.Config, error) {
	ignconfigi, err := IgnParseWrapper(rawIgn)
	if err != nil {
		return ign3types.Config{}, fmt.Errorf("failed to parse Ignition config: %w", err)
	}

	switch typedConfig := ignconfigi.(type) {
	case ign3types.Config:
		return ignconfigi.(ign3types.Config), nil
	case ign2types.Config:
		ignconfv2, err := removeIgnDuplicateFilesUnitsUsers(ignconfigi.(ign2types.Config))
		if err != nil {
			return ign3types.Config{}, err
		}
		convertedIgnV3, err := ignitionConverter.Convert(ignconfv2, *semver.New(ignconfv2.Ignition.Version), ign3types.MaxVersion)
		if err != nil {
			return ign3types.Config{}, fmt.Errorf("failed to convert Ignition config spec v2 to v3: %w", err)
		}
		return convertedIgnV3.(ign3types.Config), nil
	default:
		return ign3types.Config{}, fmt.Errorf("unexpected type for ignition config: %v", typedConfig)
	}
}
