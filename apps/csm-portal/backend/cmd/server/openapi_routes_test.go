package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestEveryRegisteredRouteIsInOpenAPI fails when a route registered in main.go
// has no operation in openapi.yaml. The gateway only forwards operations that
// the published spec documents: an undocumented route works against a local
// listener and is rejected at the gateway (a CORS preflight 404 in the browser),
// which is exactly how /projects/{id}/metadata and /tasks/search shipped broken.
func TestEveryRegisteredRouteIsInOpenAPI(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}

	param := regexp.MustCompile(`\{[^}]+\}`)
	norm := func(p string) string { return param.ReplaceAllString(p, "{}") }

	registered := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?:mux\.HandleFunc|mux\.Handle|route)\("([A-Z]+) ([^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		if m[2] == "/health" || strings.HasPrefix(m[2], "/health/") {
			continue // liveness probes are not part of the public contract
		}
		registered[m[1]+" "+norm(m[2])] = true
	}

	documented := map[string]bool{}
	var current string
	inPaths := false
	pathLine := regexp.MustCompile(`^  (/\S*):\s*$`)
	opLine := regexp.MustCompile(`^    (get|post|put|patch|delete):`)
	for _, line := range strings.Split(string(spec), "\n") {
		switch {
		case strings.HasPrefix(line, "paths:"):
			inPaths = true
		case inPaths && line != "" && line[0] != ' ' && line[0] != '#':
			inPaths = false
		case inPaths:
			if m := pathLine.FindStringSubmatch(line); m != nil {
				current = norm(m[1])
			} else if m := opLine.FindStringSubmatch(line); m != nil && current != "" {
				documented[strings.ToUpper(m[1])+" "+current] = true
			}
		}
	}

	var missing []string
	for r := range registered {
		if !documented[r] {
			missing = append(missing, r)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("routes registered in main.go but missing from openapi.yaml (the gateway will not serve them):\n  %s", strings.Join(missing, "\n  "))
	}
}
