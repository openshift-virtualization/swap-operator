package renderconfig

import (
	"testing"

	nodeswap "github.com/openshift-virtualization/swap-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestMkswapSizeArg(t *testing.T) {
	tests := []struct {
		name      string
		q         resource.Quantity
		want      string
		wantError bool
	}{
		{
			name: "1KiB -> 1Ki",
			q:    resource.MustParse("1Ki"),
			want: "1Ki",
		},
		{
			name: "4MiB -> 4Mi",
			q:    resource.MustParse("4Mi"),
			want: "4Mi",
		},
		{
			name: "2GiB -> 2Gi",
			q:    resource.MustParse("2Gi"),
			want: "2Gi",
		},
		{
			name:      "zero size -> error",
			q:         resource.MustParse("0"),
			wantError: true,
		},
		{
			name:      "negative size -> error",
			q:         resource.MustParse("-1"),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MkswapSizeArg(tt.q)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil (result=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("MkswapSizeArg(%q) = %q, want %q", tt.q.String(), got, tt.want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		name     string
		id       int
		spec     *nodeswap.SwapSpec
		wantName string
		wantErr  bool
	}{
		{
			name: "file-based swap",
			id:   0,
			spec: &nodeswap.SwapSpec{
				SwapType: nodeswap.FileBasedSwap,
				File: &nodeswap.SwapFile{
					Path: "/var/tmp/swapfile",
					Size: resource.MustParse("1Gi"),
				},
			},
			wantName: "99-filebased-swap-0",
		},
		{
			name: "disk-based swap",
			id:   3,
			spec: &nodeswap.SwapSpec{
				SwapType: nodeswap.SwapOnDisk,
				Disk: &nodeswap.SwapDisk{
					SwapPartition: nodeswap.Partition{
						PartLabel: "SWAP",
					},
				},
			},
			wantName: "99-diskbased-swap-3",
		},
		{
			name: "zram-based swap",
			id:   1,
			spec: &nodeswap.SwapSpec{
				SwapType: nodeswap.SwapOnZram,
				Zram: &nodeswap.SwapZram{
					Size: resource.MustParse("512Mi"),
				},
			},
			wantName: "99-zrambased-swap-1",
		},
		{
			name: "unknown swap type returns error",
			id:   0,
			spec: &nodeswap.SwapSpec{
				SwapType: "unknown-type",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := render(tt.id, tt.spec)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (config=%+v)", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Name != tt.wantName {
				t.Fatalf("render() Name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}
