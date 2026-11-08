// Package semver implements just enough semantic-version parsing and
// comparison to enforce contractgate's version-bump rule: a breaking
// contract change must come with a major version bump. Full semver (pre-
// release tags, build metadata) is out of scope — contract versions in
// this project are always plain MAJOR.MINOR.PATCH.
package semver

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major, Minor, Patch int
}

func Parse(s string) (Version, error) {
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("version %q must be MAJOR.MINOR.PATCH", s)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("version %q: invalid component %q", s, p)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}, nil
}

// Compare returns -1, 0, or 1 as a is less than, equal to, or greater than b.
func Compare(a, b Version) int {
	if a.Major != b.Major {
		return cmp(a.Major, b.Major)
	}
	if a.Minor != b.Minor {
		return cmp(a.Minor, b.Minor)
	}
	return cmp(a.Patch, b.Patch)
}

func cmp(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// IsMajorBump reports whether newV has a strictly greater major component
// than oldV. A major bump with a LOWER minor/patch (e.g. 1.5.0 -> 2.0.0) is
// still a valid major bump — only the major component is what signals "this
// version may break you," by this project's convention.
func IsMajorBump(oldV, newV Version) bool {
	return newV.Major > oldV.Major
}

// IsAnyBump reports whether newV is greater than oldV at all.
func IsAnyBump(oldV, newV Version) bool {
	return Compare(newV, oldV) > 0
}
