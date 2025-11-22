package template

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"text/template"

	"github.com/openshift-virtualization/swap-operator/api/v1alpha1"
	ctrlcommon "github.com/openshift-virtualization/swap-operator/internal/common"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	"k8s.io/klog/v2"
)

type RenderConfig struct {
	*v1alpha1.NodeSwapSpec
}

const (
	filesDir            = "files"
	unitsDir            = "units"
	extensionsDir       = "extensions"
	platformBase        = "_base"
	platformOnPrem      = "on-prem"
	sno                 = "sno"
	tnf                 = "two-node-with-fencing"
	masterRole          = "master"
	workerRole          = "worker"
	arbiterRole         = "arbiter"
	cloudPlatformAltDNS = "cloud-platform-alt-dns"
)

// generateTemplateMachineConfigs returns MachineConfig objects from the templateDir and a config object
// expected directory structure for correctly templating machine configs: <templatedir>/<role>/<name>/<platform>/<type>/<tmpl_file>
//
// All files from platform _base are always included, and may be overridden or
// supplemented by platform-specific templates.
//
//	ex:
//	     templates/worker/00-worker/_base/units/kubelet.conf.tmpl
//	                                  /files/hostname.tmpl
//	                            /aws/units/kubelet-dropin.conf.tmpl
//	                     /01-worker-kubelet/_base/files/random.conf.tmpl
//	              /master/00-master/_base/units/kubelet.tmpl
//	                                  /files/hostname.tmpl
func generateTemplateMachineConfigs(config *RenderConfig, templateDir string) ([]*mcfgv1.MachineConfig, error) {
	infos, err := ctrlcommon.ReadDir(templateDir)
	if err != nil {
		return nil, err
	}

	cfgs := []*mcfgv1.MachineConfig{}

	for _, info := range infos {
		if !info.IsDir() {
			klog.Infof("ignoring non-directory path %q", info.Name())
			continue
		}
		role := info.Name()

		roleConfigs, err := GenerateMachineConfigsForRole(config, role, templateDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create MachineConfig for role %s: %w", role, err)
		}
		cfgs = append(cfgs, roleConfigs...)
	}

	return cfgs, nil
}

// GenerateMachineConfigsForRole creates MachineConfigs for the role provided
func GenerateMachineConfigsForRole(config *RenderConfig, role, templateDir string) ([]*mcfgv1.MachineConfig, error) {
	rolePath := role
	//nolint:goconst
	if role != workerRole && role != masterRole && role != arbiterRole {
		// custom pools are only allowed to be worker's children
		// and can reuse the worker templates
		rolePath = workerRole
	}

	path := filepath.Join(templateDir, rolePath)
	infos, err := ctrlcommon.ReadDir(path)
	if err != nil {
		return nil, err
	}

	cfgs := []*mcfgv1.MachineConfig{}
	// This func doesn't process "common"
	// common templates are only added to 00-<role>
	// templates/<role>/{00-<role>,01-<role>-container-runtime,01-<role>-kubelet}
	for _, info := range infos {
		if !info.IsDir() {
			klog.Infof("ignoring non-directory path %q", info.Name())
			continue
		}
		name := info.Name()
		namePath := filepath.Join(path, name)
		nameConfig, err := generateMachineConfigForName(config, role, name, templateDir, namePath)
		if err != nil {
			return nil, err
		}
		cfgs = append(cfgs, nameConfig)
	}

	return cfgs, nil
}

// renderTemplate renders a template file with values from a RenderConfig
// returns the rendered file data
func renderTemplate(config RenderConfig, path string, b []byte) ([]byte, error) {
	funcs := ctrlcommon.GetTemplateFuncMap()
	tmpl, err := template.New(path).Funcs(funcs).Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", path, err)
	}

	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, config); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.Bytes(), nil
}

func filterTemplates(toFilter map[string]string, path string, config *RenderConfig) error {
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// empty templates signify don't create
		if info.Size() == 0 {
			delete(toFilter, info.Name())
			return nil
		}

		filedata, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %q: %w", path, err)
		}

		// Render the template file
		renderedData, err := renderTemplate(*config, path, filedata)
		if err != nil {
			return err
		}

		// A template may result in no data when rendered, for example if the
		// whole template is conditioned to specific values in render config.
		// The intention is there shouldn't be any resulting file or unit form
		// this template and thus we filter it here.
		// Also trim the data in case the data only consists of an extra line or space
		if len(bytes.TrimSpace(renderedData)) > 0 {
			toFilter[info.Name()] = string(renderedData)
		}

		return nil
	}

	return filepath.Walk(path, walkFn)
}

// existsDir returns true if path exists and is a directory, false if the path
// does not exist, and error if there is a runtime error or the path is not a directory
func existsDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to open dir %q: %w", path, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("expected template directory, %q is not a directory", path)
	}
	return true, nil
}

func getPaths() []string {
	platformBasedPaths := []string{platformBase}

	return platformBasedPaths
}

func generateMachineConfigForName(config *RenderConfig, role, name, templateDir, path string) (*mcfgv1.MachineConfig, error) {
	platformDirs := []string{}
	platformBasedPaths := getPaths()
	// Loop over templates/common which applies everywhere
	for _, dir := range platformBasedPaths {
		basePath := filepath.Join(templateDir, "common", dir)
		exists, err := existsDir(basePath)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		platformDirs = append(platformDirs, basePath)
	}

	// And now over the target e.g. templates/master/00-master,01-master-container-runtime,01-master-kubelet
	for _, dir := range platformBasedPaths {
		platformPath := filepath.Join(path, dir)
		exists, err := existsDir(platformPath)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		platformDirs = append(platformDirs, platformPath)
	}

	files := map[string]string{}
	units := map[string]string{}
	extensions := map[string]string{}

	// walk all role dirs, with later ones taking precedence
	for _, platformDir := range platformDirs {
		p := filepath.Join(platformDir, filesDir)
		exists, err := existsDir(p)
		if err != nil {
			return nil, err
		}
		if exists {
			if err := filterTemplates(files, p, config); err != nil {
				return nil, err
			}
		}

		p = filepath.Join(platformDir, unitsDir)
		exists, err = existsDir(p)
		if err != nil {
			return nil, err
		}
		if exists {
			if err := filterTemplates(units, p, config); err != nil {
				return nil, err
			}
		}

		p = filepath.Join(platformDir, extensionsDir)
		exists, err = existsDir(p)
		if err != nil {
			return nil, err
		}
		if exists {
			if err := filterTemplates(extensions, p, config); err != nil {
				return nil, err
			}
		}
	}

	// keySortVals returns a list of values, sorted by key
	// we need the lists of files and units to have a stable ordering for the checksum
	keySortVals := func(m map[string]string) []string {
		ks := []string{}
		for k := range m {
			ks = append(ks, k)
		}
		sort.Strings(ks)

		vs := []string{}
		for _, k := range ks {
			vs = append(vs, m[k])
		}

		return vs
	}

	ignCfg, err := ctrlcommon.TranspileCoreOSConfigToIgn(keySortVals(files), keySortVals(units))
	if err != nil {
		return nil, fmt.Errorf("error transpiling CoreOS config to Ignition config: %w", err)
	}
	mcfg, err := ctrlcommon.MachineConfigFromIgnConfig(role, name, ignCfg)
	if err != nil {
		return nil, fmt.Errorf("error creating MachineConfig from Ignition config: %w", err)
	}

	mcfg.Spec.Extensions = append(mcfg.Spec.Extensions, slices.Sorted(maps.Keys(extensions))...)

	return mcfg, nil
}
