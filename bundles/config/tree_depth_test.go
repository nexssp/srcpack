package config

import (
	"strconv"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

func TestTreeDepthDefaultAndOverrides(t *testing.T) {
	ktest.RequireEqual(t, DefaultTreeDepth, 1)
	for _, depth := range []int{-5, 0, 1, 2, 7} {
		t.Run(strconv.Itoa(depth), func(t *testing.T) {
			want := depth
			if want <= 0 {
				want = DefaultTreeDepth
			}
			ktest.RequireEqual(t, ResolveTreeDepth(depth), want)
			ktest.RequireEqual(t, (ConfigOptions{TreeDepth: &depth}).ToOptions().TreeDepth, want)
		})
	}
	ktest.RequireEqual(t, (ConfigOptions{}).ToOptions().TreeDepth, DefaultTreeDepth)
}
