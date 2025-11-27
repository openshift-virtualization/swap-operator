package renderconfig

import (
	"fmt"
	"strconv"

	nodeswap "github.com/openshift-virtualization/swap-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	FileBasedSwapMCPrefix      = "99-filebased-swap"
	DiskBasedSwapMCPrefix      = "99-diskbased-swap"
	ZramBasedSwapMCPrefix      = "99-zrambased-swap"
	SwapKubeletCgroupsMCPrefix = "99-swap-kubelet-cgroups"
)

type RenderConfig struct {
	Name                string
	Index               int
	EnableFileBasedSwap bool
	EnableDiskBasedSwap bool
	EnableZramBasedSwap bool
	SwapFileSize        string
	SwapFilePath        string
	SwapDevicePriotiry  uint
}

func Create(spec *nodeswap.NodeSwapSpec) ([]RenderConfig, error) {
	configs := []RenderConfig{}

	for idx, swap := range spec.Swaps {
		config, err := render(idx, &swap)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}

	return configs, nil
}

func render(id int, swap *nodeswap.SwapSpec) (RenderConfig, error) {
	var err error
	config := RenderConfig{Index: id}
	name, err := generateName(id, swap)
	if err != nil {
		return RenderConfig{}, err
	}

	switch swap.SwapType {
	case nodeswap.FileBasedSwap:
		config.EnableFileBasedSwap = true
		config, err = generateFileBasedSwapConfig(swap.File)
	case nodeswap.SwapOnDisk:
		config.EnableDiskBasedSwap = true
		config, err = generateDiskBasedSwapConfig(swap.Disk)
	case nodeswap.SwapOnZram:
		config.EnableZramBasedSwap = true
		config, err = generateZramBasedSwapConfig(swap.Zram)
	default:
		return RenderConfig{}, fmt.Errorf("unknown swap type: %s", swap.SwapType)
	}

	if err != nil {
		return RenderConfig{}, err
	}

	config.Name = name
	config.SwapDevicePriotiry = uint(swap.Priority)
	return config, nil
}

func generateName(id int, spec *nodeswap.SwapSpec) (string, error) {
	switch spec.SwapType {
	case nodeswap.FileBasedSwap:
		return fmt.Sprintf("%s-%d", FileBasedSwapMCPrefix, id), nil
	case nodeswap.SwapOnDisk:
		return fmt.Sprintf("%s-%d", DiskBasedSwapMCPrefix, id), nil
	case nodeswap.SwapOnZram:
		return fmt.Sprintf("%s-%d", ZramBasedSwapMCPrefix, id), nil
	}

	return "", fmt.Errorf("unknown swap type: %s", spec.SwapType)
}

func generateFileBasedSwapConfig(swapConfig *nodeswap.SwapFile) (RenderConfig, error) {
	size, err := MkswapSizeArg(swapConfig.Size)
	if err != nil {
		return RenderConfig{}, err
	}

	return RenderConfig{
		SwapFileSize: size,
		SwapFilePath: swapConfig.Path,
	}, nil
}

func generateDiskBasedSwapConfig(swapConfig *nodeswap.SwapDisk) (RenderConfig, error) {
	return RenderConfig{}, nil
}

func generateZramBasedSwapConfig(swapConfig *nodeswap.SwapZram) (RenderConfig, error) {
	return RenderConfig{}, nil
}

// MkswapSizeArg converts a resource.Quantity to a string suitable for fallocate --length.
// It prefers an integer GiB/MiB/KiB suffix (Gi/Mi/Ki). If the size does not divide
// evenly by 1024, it falls back to raw bytes.
func MkswapSizeArg(q resource.Quantity) (string, error) {
	if q.Sign() <= 0 {
		return "", fmt.Errorf("swap size must be positive, got %s", q.String())
	}

	bytes := q.Value() // memory-like quantities are in bytes
	if bytes <= 0 {
		return "", fmt.Errorf("swap size must be positive in bytes, got %d", bytes)
	}

	type unit struct {
		name string
		size int64
	}

	units := []unit{
		{"Gi", 1024 * 1024 * 1024},
		{"Mi", 1024 * 1024},
		{"Ki", 1024},
	}

	// Try Gi, then Mi, then Ki
	for _, u := range units {
		if bytes%u.size == 0 {
			return strconv.FormatInt(bytes/u.size, 10) + u.name, nil
		}
	}

	// Fallback: raw bytes
	return "", fmt.Errorf("swap size must be in Ki Mi or Gi formats, got %d", bytes)
}
