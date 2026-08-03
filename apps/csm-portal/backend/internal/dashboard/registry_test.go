// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// writeDefinition writes one dashboard definition file into dir and returns
// its path. name is the filename, which the loader must treat as meaningless.
func writeDefinition(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

const csDefinition = `{
  "id": "cs-overview",
  "displayName": "CS Overview",
  "type": "cs",
  "isDefault": true,
  "widgets": [
    {"id": "open-cases", "displayName": "Open Cases", "resourceType": "case", "shape": "count", "gridWidth": 3,
     "query": {"filters": [{"field": "state", "op": "in", "values": ["open"]}]}}
  ]
}`

const creDefinition = `{
  "id": "cre-team",
  "displayName": "CRE Team",
  "type": "cre",
  "isTeamBased": true,
  "targetTeam": "abt",
  "widgets": [
    {"id": "team-cases", "displayName": "Team Cases", "resourceType": "case", "shape": "count", "gridWidth": 3,
     "query": {"filters": [{"field": "state", "op": "in", "values": ["open"]}]}},
    {"id": "team-p1", "displayName": "Team P1", "resourceType": "case", "shape": "count", "gridWidth": 3,
     "query": {"filters": [{"field": "severity", "op": "in", "values": ["critical"]}]}}
  ]
}`

func TestLoadDir_HappyPath(t *testing.T) {
	dir := t.TempDir()
	// Deliberately unhelpful filenames: the loader must take id, displayName
	// and type from the content, never from the name, and must order by
	// filename so the result is deterministic.
	writeDefinition(t, dir, "02-second.json", creDefinition)
	writeDefinition(t, dir, "01-first.json", csDefinition)
	// Non-JSON siblings are ignored rather than erroring: the directory is
	// hand-maintained and will collect READMEs and editor droppings.
	writeDefinition(t, dir, "README.md", "not a dashboard")
	writeDefinition(t, dir, "notes.txt", "also not a dashboard")
	if err := os.Mkdir(filepath.Join(dir, "archive"), 0o750); err != nil {
		t.Fatalf("mkdir archive: %v", err)
	}

	got, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("LoadDir returned %d dashboards, want 2: %+v", len(got), got)
	}
	if got[0].ID != "cs-overview" || got[1].ID != "cre-team" {
		t.Fatalf("dashboard order = %q, %q; want cs-overview then cre-team (lexical filename order)", got[0].ID, got[1].ID)
	}
	if got[0].Type != TypeCS || !got[0].IsDefault || got[0].IsTeamBased {
		t.Fatalf("cs-overview = %+v; want type cs, isDefault, not team based", got[0])
	}
	if got[1].Type != TypeCRE || !got[1].IsTeamBased || got[1].TargetTeam != "abt" {
		t.Fatalf("cre-team = %+v; want type cre, team based, targetTeam abt", got[1])
	}
	if len(got[1].Widgets) != 2 {
		t.Fatalf("cre-team has %d widgets, want 2", len(got[1].Widgets))
	}
}

func TestLoadDir_EmptyDirectory(t *testing.T) {
	got, err := LoadDir(t.TempDir())
	if err != nil {
		t.Fatalf("LoadDir(empty dir) returned error: %v; an empty directory is legal", err)
	}
	if len(got) != 0 {
		t.Fatalf("LoadDir(empty dir) = %+v, want no dashboards", got)
	}
}

func TestLoadDir_MissingDirectoryIsAnError(t *testing.T) {
	_, err := LoadDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("LoadDir(missing dir) returned no error; a misconfigured path must fail the deploy")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Fatalf("error %q does not name the offending directory", err)
	}
}

// TestLoadDir_MalformedFileFailsNamingIt is the central guarantee: a broken
// definition must never be skipped, because a skipped dashboard is invisible.
func TestLoadDir_MalformedFileFailsNamingIt(t *testing.T) {
	dir := t.TempDir()
	writeDefinition(t, dir, "good.json", csDefinition)
	writeDefinition(t, dir, "broken.json", `{"id": "broken", "displayName":`)

	got, err := LoadDir(dir)
	if err == nil {
		t.Fatalf("LoadDir returned no error for a malformed file; got %+v", got)
	}
	if !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("error %q does not name the offending file", err)
	}
	if got != nil {
		t.Fatalf("LoadDir returned %+v alongside the error; a partial load must not be served", got)
	}
}

func TestLoadDir_UnreadableFileFailsNamingIt(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: file permissions do not deny reads")
	}
	dir := t.TempDir()
	path := writeDefinition(t, dir, "locked.json", csDefinition)
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("LoadDir returned no error for an unreadable file")
	}
	if !strings.Contains(err.Error(), "locked.json") {
		t.Fatalf("error %q does not name the offending file", err)
	}
}

func TestLoadDir_DuplicateIDFailsNamingBothFiles(t *testing.T) {
	dir := t.TempDir()
	writeDefinition(t, dir, "a.json", csDefinition)
	writeDefinition(t, dir, "b.json", csDefinition)

	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("LoadDir returned no error for two files sharing one dashboard id")
	}
	for _, want := range []string{"a.json", "b.json", "cs-overview"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

func TestLoadDir_MissingIDFails(t *testing.T) {
	dir := t.TempDir()
	writeDefinition(t, dir, "anonymous.json", `{"displayName": "No Id", "type": "cs", "widgets": []}`)

	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("LoadDir returned no error for a definition with no id")
	}
	if !strings.Contains(err.Error(), "anonymous.json") {
		t.Fatalf("error %q does not name the offending file", err)
	}
}

func TestLoadDir_MissingDisplayNameFails(t *testing.T) {
	dir := t.TempDir()
	writeDefinition(t, dir, "nameless.json", `{"id": "nameless", "type": "cs", "widgets": []}`)

	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("LoadDir returned no error for a definition with no displayName")
	}
	if !strings.Contains(err.Error(), "nameless.json") {
		t.Fatalf("error %q does not name the offending file", err)
	}
}

func TestLoadDir_TypeIsRequiredAndClosed(t *testing.T) {
	t.Run("missing type", func(t *testing.T) {
		dir := t.TempDir()
		writeDefinition(t, dir, "untyped.json", `{"id": "untyped", "displayName": "Untyped", "widgets": []}`)
		_, err := LoadDir(dir)
		if err == nil || !strings.Contains(err.Error(), "untyped.json") {
			t.Fatalf("LoadDir error = %v; want a rejection naming untyped.json", err)
		}
	})

	t.Run("unknown type", func(t *testing.T) {
		dir := t.TempDir()
		writeDefinition(t, dir, "weird.json", `{"id": "weird", "displayName": "Weird", "type": "ops", "widgets": []}`)
		_, err := LoadDir(dir)
		if err == nil {
			t.Fatal("LoadDir returned no error for an unknown dashboard type")
		}
		for _, want := range []string{"weird.json", "ops"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q does not mention %q", err, want)
			}
		}
	})

	t.Run("every valid type is accepted", func(t *testing.T) {
		for _, tc := range []struct {
			typ         Type
			isTeamBased bool
		}{
			{TypeCRE, true},
			{TypeSRE, true},
			{TypeCS, false},
		} {
			dir := t.TempDir()
			writeDefinition(t, dir, "d.json", `{"id": "d", "displayName": "D", "type": "`+string(tc.typ)+
				`", "isTeamBased": `+boolLiteral(tc.isTeamBased)+`, "widgets": []}`)
			got, err := LoadDir(dir)
			if err != nil {
				t.Fatalf("LoadDir(type %q) returned error: %v", tc.typ, err)
			}
			if len(got) != 1 || got[0].Type != tc.typ {
				t.Fatalf("LoadDir(type %q) = %+v", tc.typ, got)
			}
		}
	})
}

func boolLiteral(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// TestLoadDir_ContradictoryCombinations pins down exactly which
// type/isDefault/isTeamBased combinations are rejected. The three fields are
// independent by product decision, which is precisely why they need this.
func TestLoadDir_ContradictoryCombinations(t *testing.T) {
	cases := []struct {
		name    string
		files   map[string]string
		wantErr []string
	}{
		{
			name: "team-scoped cre type with isTeamBased false",
			files: map[string]string{
				"a.json": `{"id": "a", "displayName": "A", "type": "cre", "isTeamBased": false, "widgets": []}`,
			},
			wantErr: []string{"a.json", "cre", "isTeamBased"},
		},
		{
			name: "team-scoped sre type with isTeamBased omitted (defaults false)",
			files: map[string]string{
				"a.json": `{"id": "a", "displayName": "A", "type": "sre", "widgets": []}`,
			},
			wantErr: []string{"a.json", "sre", "isTeamBased"},
		},
		{
			name: "organisation-wide cs type with isTeamBased true",
			files: map[string]string{
				"a.json": `{"id": "a", "displayName": "A", "type": "cs", "isTeamBased": true, "widgets": []}`,
			},
			wantErr: []string{"a.json", "cs", "isTeamBased"},
		},
		{
			name: "two defaults of the same type",
			files: map[string]string{
				"a.json": `{"id": "a", "displayName": "A", "type": "cs", "isDefault": true, "widgets": []}`,
				"b.json": `{"id": "b", "displayName": "B", "type": "cs", "isDefault": true, "widgets": []}`,
			},
			wantErr: []string{"a.json", "b.json", "isDefault"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range tc.files {
				writeDefinition(t, dir, name, body)
			}
			_, err := LoadDir(dir)
			if err == nil {
				t.Fatal("LoadDir returned no error, want a rejection")
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

// TestLoadDir_DefaultsOfDifferentTypesCoexist is the counterpart: one default
// per type is the normal, required arrangement.
func TestLoadDir_DefaultsOfDifferentTypesCoexist(t *testing.T) {
	dir := t.TempDir()
	writeDefinition(t, dir, "a.json", `{"id": "a", "displayName": "A", "type": "cs", "isDefault": true, "widgets": []}`)
	writeDefinition(t, dir, "b.json", `{"id": "b", "displayName": "B", "type": "cre", "isDefault": true, "isTeamBased": true, "widgets": []}`)
	writeDefinition(t, dir, "c.json", `{"id": "c", "displayName": "C", "type": "sre", "isDefault": true, "isTeamBased": true, "widgets": []}`)

	got, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("LoadDir returned %d dashboards, want 3", len(got))
	}
}

// TestLoadDir_MigratesLegacyWidgetKeys proves the deprecated-key migration
// runs on directory-loaded definitions too, not just on DASHBOARDS_CONFIG.
func TestLoadDir_MigratesLegacyWidgetKeys(t *testing.T) {
	dir := t.TempDir()
	writeDefinition(t, dir, "legacy.json", `{
	  "id": "legacy", "displayName": "Legacy", "type": "cs",
	  "widgets": [
	    {"id": "w", "displayName": "W", "resourceType": "case", "shape": "count", "gridWidth": 3,
	     "filters": {"orGroups": [[{"field": "state", "op": "in", "values": ["open"]}]]}}
	  ]
	}`)

	got, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	query := got[0].Widgets[0].Query
	if query == nil {
		t.Fatal("widget Query is nil; the legacy \"filters\" key was not migrated")
	}
	if _, ok := query["orGroups"]; ok {
		t.Fatalf("widget Query still carries \"orGroups\": %+v", query)
	}
	branches, ok := query["anyOf"].([]any)
	if !ok || len(branches) != 1 {
		t.Fatalf("widget Query anyOf = %+v, want one migrated branch", query["anyOf"])
	}
	if _, ok := branches[0].(map[string]any)["filters"]; !ok {
		t.Fatalf("migrated branch = %+v, want it wrapped as {\"filters\": [...]}", branches[0])
	}
}

// TestRegistry_DefaultModeReadsDiskExactlyOnce is the whole point of the
// default mode: the startup read is the only read, no matter how many
// requests come in.
func TestRegistry_DefaultModeReadsDiskExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	writeDefinition(t, dir, "a.json", csDefinition)

	var reads atomic.Int64
	r, err := NewDirRegistry(dir, false)
	if err != nil {
		t.Fatalf("NewDirRegistry returned error: %v", err)
	}
	// Swap in a counting loader AFTER construction, so the count covers only
	// post-startup reads. It must stay at zero.
	r.load = func(d string) ([]Dashboard, error) {
		reads.Add(1)
		return LoadDir(d)
	}

	for i := 0; i < 5; i++ {
		if got := r.Dashboards(); len(got) != 1 {
			t.Fatalf("read %d: got %d dashboards, want 1", i, len(got))
		}
		if _, ok := r.ByID("cs-overview"); !ok {
			t.Fatalf("read %d: cs-overview not found", i)
		}
	}
	if n := reads.Load(); n != 0 {
		t.Fatalf("the registry touched the disk %d times after startup; the default mode must read exactly once", n)
	}

	// The strongest form of the same claim: with the directory gone entirely,
	// the in-memory copy still serves.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove dir: %v", err)
	}
	if got := r.Dashboards(); len(got) != 1 {
		t.Fatalf("after deleting the directory, got %d dashboards, want the 1 held in memory", len(got))
	}
}

// TestRegistry_HotReloadPicksUpChanges covers the local-development mode:
// editing a definition after startup is visible without a restart.
func TestRegistry_HotReloadPicksUpChanges(t *testing.T) {
	dir := t.TempDir()
	writeDefinition(t, dir, "a.json", csDefinition)

	r, err := NewDirRegistry(dir, true)
	if err != nil {
		t.Fatalf("NewDirRegistry returned error: %v", err)
	}
	if got := r.Dashboards(); len(got) != 1 || got[0].DisplayName != "CS Overview" {
		t.Fatalf("initial read = %+v", got)
	}

	// Edit an existing definition.
	writeDefinition(t, dir, "a.json", strings.Replace(csDefinition, "CS Overview", "CS Overview v2", 1))
	got := r.Dashboards()
	if len(got) != 1 || got[0].DisplayName != "CS Overview v2" {
		t.Fatalf("after editing the file, read = %+v; want the new displayName", got)
	}

	// Add a whole new definition file.
	writeDefinition(t, dir, "b.json", creDefinition)
	if got := r.Dashboards(); len(got) != 2 {
		t.Fatalf("after adding a file, got %d dashboards, want 2", len(got))
	}

	// Remove one again.
	if err := os.Remove(filepath.Join(dir, "b.json")); err != nil {
		t.Fatalf("remove b.json: %v", err)
	}
	if got := r.Dashboards(); len(got) != 1 {
		t.Fatalf("after removing a file, got %d dashboards, want 1", len(got))
	}
}

// TestRegistry_HotReloadKeepsLastKnownGoodOnError is the deliberate asymmetry
// with the startup path. Startup fails hard on a bad definition set; a
// running dev server does not, because the overwhelmingly common cause is an
// editor writing a half-finished JSON file mid-keystroke. It logs loudly and
// keeps serving what last parsed, and recovers by itself once the file is
// valid again.
func TestRegistry_HotReloadKeepsLastKnownGoodOnError(t *testing.T) {
	dir := t.TempDir()
	writeDefinition(t, dir, "a.json", csDefinition)

	r, err := NewDirRegistry(dir, true)
	if err != nil {
		t.Fatalf("NewDirRegistry returned error: %v", err)
	}

	writeDefinition(t, dir, "a.json", `{"id": "cs-overview", "displayName":`)
	got := r.Dashboards()
	if len(got) != 1 || got[0].DisplayName != "CS Overview" {
		t.Fatalf("with a mid-save file on disk, read = %+v; want the last known-good definitions", got)
	}

	// A contradictory (but well-formed) edit is held back the same way.
	writeDefinition(t, dir, "a.json", `{"id": "cs-overview", "displayName": "CS", "type": "cs", "isTeamBased": true, "widgets": []}`)
	if got := r.Dashboards(); len(got) != 1 || got[0].DisplayName != "CS Overview" {
		t.Fatalf("with a contradictory file on disk, read = %+v; want the last known-good definitions", got)
	}

	// And it recovers on its own once the file parses again.
	writeDefinition(t, dir, "a.json", strings.Replace(csDefinition, "CS Overview", "CS Overview fixed", 1))
	if got := r.Dashboards(); len(got) != 1 || got[0].DisplayName != "CS Overview fixed" {
		t.Fatalf("after the file was fixed, read = %+v; want the repaired definitions", got)
	}
}

// TestNewDirRegistry_FailsAtStartupInBothModes: hot-reload must not soften
// the startup contract. A broken definition set is a broken deploy either
// way.
func TestNewDirRegistry_FailsAtStartupInBothModes(t *testing.T) {
	for _, hotReload := range []bool{false, true} {
		dir := t.TempDir()
		writeDefinition(t, dir, "broken.json", `{"id": "broken", "displayName":`)
		if _, err := NewDirRegistry(dir, hotReload); err == nil {
			t.Fatalf("NewDirRegistry(hotReload=%v) returned no error for a malformed definition", hotReload)
		}
	}
}

// TestNilRegistry_ServesNothing guards the pre-startup / unconfigured case:
// the handlers must degrade to an empty list rather than panicking.
func TestNilRegistry_ServesNothing(t *testing.T) {
	var r *Registry
	if got := r.Dashboards(); got != nil {
		t.Fatalf("(*Registry)(nil).Dashboards() = %+v, want nil", got)
	}
	if _, ok := r.ByID("anything"); ok {
		t.Fatal("(*Registry)(nil).ByID returned ok=true")
	}
}

// TestParseDashboardsConfig_ToleratesMissingType: the deprecated single-
// variable path predates the type field entirely, so requiring one there
// would break every already-deployed value. It warns instead. The
// contradiction rules still apply once a type IS set.
func TestParseDashboardsConfig_ToleratesMissingType(t *testing.T) {
	got, err := ParseDashboardsConfig(`[{"id":"a","displayName":"A","widgets":[]}]`)
	if err != nil {
		t.Fatalf("ParseDashboardsConfig returned error: %v", err)
	}
	if len(got) != 1 || got[0].Type != "" {
		t.Fatalf("ParseDashboardsConfig = %+v, want one dashboard with no type", got)
	}
}

func TestParseDashboardsConfig_RejectsContradictions(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"cre type not team based", `[{"id":"a","displayName":"A","type":"cre","isTeamBased":false,"widgets":[]}]`},
		{"cs type team based", `[{"id":"a","displayName":"A","type":"cs","isTeamBased":true,"widgets":[]}]`},
		{"unknown type", `[{"id":"a","displayName":"A","type":"ops","widgets":[]}]`},
		{"duplicate id", `[{"id":"a","displayName":"A","widgets":[]},{"id":"a","displayName":"B","widgets":[]}]`},
		{"empty id", `[{"id":"","displayName":"A","widgets":[]}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseDashboardsConfig(tc.raw); err == nil {
				t.Fatal("ParseDashboardsConfig returned no error, want a rejection")
			}
		})
	}
}
