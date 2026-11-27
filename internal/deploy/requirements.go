package deploy

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Requirements are what a stack needs of the node it runs on, read from
// its install script's minimums and its compose file's reservations.
type Requirements struct {
	// Where the minimums come from, e.g. install/_min-requirements.sh
	Source string `json:"source,omitempty"`
	// The first variant whose condition holds applies; the last has none
	Variants      []RequirementVariant `json:"variants,omitempty"`
	DockerVersion string               `json:"docker_version,omitempty"`
	// What the compose file reserves, summed over its services
	ReservedMemoryMB int     `json:"reserved_memory_mb,omitempty"`
	ReservedCPUs     float64 `json:"reserved_cpus,omitempty"`
}

type RequirementVariant struct {
	When     *Condition `json:"when,omitempty"`
	MemoryMB int        `json:"memory_mb,omitempty"`
	CPUs     float64    `json:"cpus,omitempty"`
}

// Condition is a script's [[ "$VAR" == "value" ]], or != with Not.
type Condition struct {
	Variable string `json:"variable"`
	Value    string `json:"value"`
	Not      bool   `json:"not,omitempty"`
}

func (c *Condition) holds(vars map[string]string) bool {
	return c == nil || (vars[c.Variable] == c.Value) != c.Not
}

// Applicable returns the minimums that hold for these variables.
func (r *Requirements) Applicable(vars map[string]string) RequirementVariant {
	for _, v := range r.Variants {
		if v.When.holds(vars) {
			return v
		}
	}
	return RequirementVariant{}
}

func (r *Requirements) empty() bool {
	return len(r.Variants) == 0 && r.DockerVersion == "" && r.ReservedMemoryMB == 0 && r.ReservedCPUs == 0
}

var (
	minAssign = regexp.MustCompile(`^(?:export\s+|readonly\s+|local\s+)?MIN_(RAM|MEM|MEMORY|CPU|CPUS|DOCKER_VERSION)(?:_HARD)?=['"]?([0-9][0-9.]*)['"]?\s*(?:#.*)?$`)
	ifTest    = regexp.MustCompile(`^if \[\[? *"?\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?"? *(==|=|!=) *['"]?([^'"\] ]*)['"]? *\]\]?; *then$`)
)

// scriptRequirements reads the minimums an install script and the scripts
// next to it declare, as Sentry's install/_min-requirements.sh does.
func scriptRequirements(projectDir string) (Requirements, bool) {
	files := []string{}
	for _, name := range []string{"install.sh", "setup.sh"} {
		if info, err := os.Stat(filepath.Join(projectDir, name)); err == nil && !info.IsDir() {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		return Requirements{}, false
	}
	for _, dir := range []string{"install", "scripts", "setup"} {
		matches, _ := filepath.Glob(filepath.Join(projectDir, dir, "*.sh"))
		for _, m := range matches {
			rel, _ := filepath.Rel(projectDir, m)
			files = append(files, filepath.ToSlash(rel))
		}
	}
	for _, f := range files {
		if req, ok := parseRequirements(readText(filepath.Join(projectDir, f))); ok {
			req.Source = f
			return req, true
		}
	}
	return Requirements{}, false
}

// parseRequirements reads MIN_RAM, MIN_CPU and MIN_DOCKER_VERSION
// assignments, at the top level or in one if/else on a variable.
func parseRequirements(text string) (Requirements, bool) {
	var req Requirements
	// The branches of the if being read: its condition, then its else
	type branch struct {
		cond *Condition
		v    RequirementVariant
		set  bool
	}
	var base RequirementVariant
	baseSet := false
	var branches []branch
	depth := 0
	// Inside an if whose condition can't be read, or nested deeper
	opaque := false

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "if "):
			depth++
			if depth == 1 {
				if m := ifTest.FindStringSubmatch(line); m != nil {
					branches = []branch{{cond: &Condition{Variable: m[1], Value: m[3], Not: m[2] == "!="}}}
					opaque = false
				} else {
					opaque = true
				}
			}
			continue
		case line == "fi" || strings.HasPrefix(line, "fi "):
			depth--
			if depth == 0 {
				if !opaque && len(branches) > 0 {
					for _, b := range branches {
						if !b.set {
							continue
						}
						v := b.v
						v.When = b.cond
						req.Variants = append(req.Variants, v)
					}
				}
				branches, opaque = nil, false
			}
			continue
		case depth == 1 && line == "else":
			if !opaque && len(branches) == 1 {
				c := *branches[0].cond
				c.Not = !c.Not
				branches = append(branches, branch{cond: &c})
			} else {
				opaque = true
			}
			continue
		case depth == 1 && strings.HasPrefix(line, "elif "):
			opaque = true
			continue
		}

		m := minAssign.FindStringSubmatch(line)
		if m == nil || depth > 1 || (depth == 1 && opaque) {
			continue
		}
		if m[1] == "DOCKER_VERSION" {
			req.DockerVersion = m[2]
			continue
		}
		var v *RequirementVariant
		if depth == 0 {
			v, baseSet = &base, true
		} else {
			b := &branches[len(branches)-1]
			v, b.set = &b.v, true
		}
		n, _ := strconv.ParseFloat(m[2], 64)
		switch m[1] {
		case "RAM", "MEM", "MEMORY":
			v.MemoryMB = int(n)
		default:
			v.CPUs = n
		}
	}

	if baseSet {
		// A branch that only sets one of them keeps the other from the base
		for i := range req.Variants {
			if req.Variants[i].MemoryMB == 0 {
				req.Variants[i].MemoryMB = base.MemoryMB
			}
			if req.Variants[i].CPUs == 0 {
				req.Variants[i].CPUs = base.CPUs
			}
		}
		req.Variants = append(req.Variants, base)
	}
	// An if/else covers every case; mark the else as the fallback
	if n := len(req.Variants); n > 0 && !baseSet && n == 2 && req.Variants[1].When != nil &&
		req.Variants[0].When != nil && req.Variants[1].When.Not != req.Variants[0].When.Not {
		req.Variants[1].When = nil
	}
	return req, len(req.Variants) > 0 || req.DockerVersion != ""
}

// composeReservations sums the memory and CPUs services reserve, from
// deploy.resources.reservations or mem_reservation.
func composeReservations(services map[string]map[string]any) (memoryMB int, cpus float64) {
	var bytes int64
	for _, svc := range services {
		reservations := map[string]any{}
		if d, ok := svc["deploy"].(map[string]any); ok {
			if r, ok := d["resources"].(map[string]any); ok {
				reservations, _ = r["reservations"].(map[string]any)
			}
		}
		mem := reservations["memory"]
		if mem == nil {
			mem = svc["mem_reservation"]
		}
		if n, ok := parseBytes(mem); ok {
			bytes += n
		}
		if c, ok := parseNumber(reservations["cpus"]); ok {
			cpus += c
		}
	}
	return int(bytes / (1 << 20)), math.Round(cpus*100) / 100
}

var byteSize = regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)\s*([kmgt]?)i?b?$`)

// parseBytes reads compose sizes: 512m, 1.5g, 1gb, or a number of bytes.
func parseBytes(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	case string:
		m := byteSize.FindStringSubmatch(strings.TrimSpace(n))
		if m == nil {
			return 0, false
		}
		f, _ := strconv.ParseFloat(m[1], 64)
		shift := strings.Index("kmgt", strings.ToLower(m[2])) + 1
		if m[2] == "" {
			shift = 0
		}
		return int64(f * float64(int64(1)<<(10*shift))), true
	}
	return 0, false
}

func parseNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	}
	return 0, false
}

// versionAtLeast compares dotted versions like 27.3.1 and 19.03.6.
func versionAtLeast(have, want string) bool {
	h := strings.Split(strings.TrimPrefix(strings.SplitN(have, "-", 2)[0], "v"), ".")
	w := strings.Split(want, ".")
	for i := 0; i < len(w); i++ {
		var a, b int
		if i < len(h) {
			a, _ = strconv.Atoi(h[i])
		}
		b, _ = strconv.Atoi(w[i])
		if a != b {
			return a > b
		}
	}
	return true
}

func formatMemory(mb int) string {
	if mb >= 1024 {
		gb := float64(mb) / 1024
		if gb == math.Trunc(gb) || gb >= 10 {
			return fmt.Sprintf("%.0f GB", gb)
		}
		return fmt.Sprintf("%.1f GB", gb)
	}
	return fmt.Sprintf("%d MB", mb)
}
