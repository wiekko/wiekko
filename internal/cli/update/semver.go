package update

import (
	"strconv"
	"strings"
)

// version is a parsed SemVer version with a leading "v".
type version struct {
	core  [3]uint64
	pre   []string
	build string
}

func parse(v string) (version, bool) {
	var p version
	rest, ok := strings.CutPrefix(v, "v")
	if !ok {
		return p, false
	}
	rest, p.build, _ = strings.Cut(rest, "+")
	core, pre, hasPre := strings.Cut(rest, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return p, false
	}
	for i, s := range parts {
		if !numeric(s) || (len(s) > 1 && s[0] == '0') {
			return p, false
		}
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return p, false
		}
		p.core[i] = n
	}
	if hasPre {
		p.pre = strings.Split(pre, ".")
		for _, id := range p.pre {
			if !identifier(id) {
				return p, false
			}
		}
	}
	if p.build != "" {
		for _, id := range strings.Split(p.build, ".") {
			if !identifier(id) {
				return p, false
			}
		}
	}
	return p, true
}

func numeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func identifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-') {
			return false
		}
	}
	return true
}

// Valid reports whether v is a release version: vMAJOR.MINOR.PATCH with an
// optional pre-release suffix, but no build metadata (dev builds carry
// "+dev…", see internal/buildinfo).
func Valid(v string) bool {
	p, ok := parse(v)
	return ok && p.build == ""
}

// Final reports whether v is a release version without pre-release suffix.
func Final(v string) bool {
	p, ok := parse(v)
	return ok && p.build == "" && len(p.pre) == 0
}

// Compare orders two versions by SemVer precedence, ignoring build metadata:
// -1 if a < b, 0 if equal, +1 if a > b. An invalid version sorts before every
// valid one.
func Compare(a, b string) int {
	pa, oka := parse(a)
	pb, okb := parse(b)
	switch {
	case !oka && !okb:
		return strings.Compare(a, b)
	case !oka:
		return -1
	case !okb:
		return 1
	}
	for i := range pa.core {
		if c := cmpUint(pa.core[i], pb.core[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(pa.pre) == 0 && len(pb.pre) == 0:
		return 0
	case len(pa.pre) == 0:
		return 1 // a release ranks above its pre-releases
	case len(pb.pre) == 0:
		return -1
	}
	for i := 0; i < len(pa.pre) && i < len(pb.pre); i++ {
		if c := cmpIdentifier(pa.pre[i], pb.pre[i]); c != 0 {
			return c
		}
	}
	return cmpUint(uint64(len(pa.pre)), uint64(len(pb.pre)))
}

func cmpIdentifier(a, b string) int {
	na, nb := numeric(a), numeric(b)
	switch {
	case na && nb:
		x, _ := strconv.ParseUint(a, 10, 64)
		y, _ := strconv.ParseUint(b, 10, 64)
		return cmpUint(x, y)
	case na:
		return -1 // numeric identifiers rank below alphanumeric ones
	case nb:
		return 1
	}
	return strings.Compare(a, b)
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
