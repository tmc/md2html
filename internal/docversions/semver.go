package docversions

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// Compare orders two version names. Names that parse as semantic
// versions, with or without a leading "v", compare by semantic version
// precedence, so v0.1.10 is above v0.1.9 and v1.0.0-rc.1 is below
// v1.0.0. A semantic version is above any name that is not one, and
// such names compare as strings. The result is -1, 0, or +1.
func Compare(a, b string) int {
	va, oka := parse(a)
	vb, okb := parse(b)
	switch {
	case oka && okb:
		if c := compareSemver(va, vb); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	case oka:
		return +1
	case okb:
		return -1
	}
	return strings.Compare(a, b)
}

// SortNewestFirst sorts names by [Compare], highest first.
func SortNewestFirst(names []string) {
	slices.SortFunc(names, func(a, b string) int { return Compare(b, a) })
}

type semver struct {
	nums [3]int
	pre  []string
}

// parse parses a version such as v1.2.3-rc.1+build. Minor and patch
// may be omitted, as in v1 or v1.2.
func parse(s string) (semver, bool) {
	var v semver
	s = strings.TrimPrefix(s, "v")
	s, _, _ = strings.Cut(s, "+")
	s, pre, hasPre := strings.Cut(s, "-")
	if hasPre {
		if pre == "" {
			return v, false
		}
		v.pre = strings.Split(pre, ".")
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return v, false
		}
		v.nums[i] = n
	}
	return v, true
}

func compareSemver(a, b semver) int {
	for i := range a.nums {
		if c := cmp.Compare(a.nums[i], b.nums[i]); c != 0 {
			return c
		}
	}
	// A prerelease is below the release it precedes.
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return +1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := comparePrerelease(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a.pre), len(b.pre))
}

// comparePrerelease compares prerelease identifiers: numeric ones
// numerically and below alphanumeric ones, which compare as strings.
func comparePrerelease(a, b string) int {
	na, erra := strconv.Atoi(a)
	nb, errb := strconv.Atoi(b)
	switch {
	case erra == nil && errb == nil:
		return cmp.Compare(na, nb)
	case erra == nil:
		return -1
	case errb == nil:
		return +1
	}
	return strings.Compare(a, b)
}
