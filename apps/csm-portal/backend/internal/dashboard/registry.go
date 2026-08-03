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
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Type classifies a dashboard by the audience it is built for. The frontend
// keys its automatic dashboard selection off this: a caller whose team family
// is cre-abt/cre lands on the default TypeCRE dashboard, sre-abt/sre on the
// default TypeSRE one, and a caller with no team at all on the default
// TypeCS one.
type Type string

const (
	// TypeCRE is a Customer Renewal & Expansion team dashboard. Team-scoped.
	TypeCRE Type = "cre"
	// TypeSRE is a Site Reliability Engineering team dashboard. Team-scoped.
	TypeSRE Type = "sre"
	// TypeCS is a CS-organisation-wide dashboard, not scoped to a team.
	TypeCS Type = "cs"
)

var validTypes = map[Type]bool{TypeCRE: true, TypeSRE: true, TypeCS: true}

// definitionExt is the only file extension the definition directory loader
// considers. Everything else in the directory (README, .yaml, editor swap
// files, subdirectories) is ignored rather than erroring: the directory is
// expected to be hand-maintained.
const definitionExt = ".json"

// sourced pairs a decoded dashboard with where it came from, so every
// validation error can name the offending file (or the offending index in the
// deprecated single-variable config) instead of just an id.
type sourced struct {
	dashboard Dashboard
	source    string
}

// Registry holds the dashboard definitions a running process serves.
//
// Two modes, chosen at construction:
//
//   - default (hotReload false): the definitions are read once, at startup,
//     and every subsequent read is served from memory. There is no disk I/O
//     on the request path at all. This is the deployed behaviour.
//   - hot-reload (hotReload true): every read re-reads the directory, so
//     editing a definition file is picked up without restarting. Intended for
//     local development only; it puts a directory scan plus a file read per
//     definition on every request.
type Registry struct {
	mu     sync.RWMutex
	cached []Dashboard

	dir       string
	hotReload bool

	// load is the directory reader, injectable so tests can count how many
	// times the registry actually touches the disk. nil for a static
	// registry.
	load func(dir string) ([]Dashboard, error)
}

// NewStaticRegistry wraps an already-decoded set of dashboards. It never
// touches the disk. Used by tests and by the deprecated DASHBOARDS_CONFIG
// path, which has no directory to re-read.
func NewStaticRegistry(dashboards []Dashboard) *Registry {
	return &Registry{cached: append([]Dashboard(nil), dashboards...)}
}

// NewDirRegistry builds a registry over a directory of per-dashboard JSON
// files. It always performs the initial load eagerly and returns the error,
// in both modes: a definition set that is broken at startup is a broken
// deploy, and the caller is expected to make it fatal. hotReload only governs
// what happens on subsequent reads.
func NewDirRegistry(dir string, hotReload bool) (*Registry, error) {
	r := &Registry{dir: dir, hotReload: hotReload, load: LoadDir}
	dashboards, err := r.load(dir)
	if err != nil {
		return nil, err
	}
	r.cached = dashboards
	return r, nil
}

// Dashboards returns the registry's dashboards in their deterministic order.
//
// In hot-reload mode a failed re-read does NOT take the service down and does
// NOT start returning an empty list: it logs the error loudly and keeps
// serving the last known-good set. The startup load has already proven the
// directory is valid, so a failure here is almost always an editor mid-save
// writing half a JSON file, and killing a dev server (or blanking its
// dashboards) on every keystroke would make the loop unusable. The loud log
// keeps the failure visible, which is the part that matters. The startup path
// is unaffected and still fails hard.
func (r *Registry) Dashboards() []Dashboard {
	if r == nil {
		return nil
	}
	if r.hotReload && r.load != nil {
		if dashboards, err := r.load(r.dir); err != nil {
			slog.Error("dashboard definitions: hot reload failed; continuing to serve the last known-good definitions",
				"dir", r.dir, "err", err)
		} else {
			r.mu.Lock()
			r.cached = dashboards
			r.mu.Unlock()
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cached
}

// ByID looks a dashboard up by id, returning ok=false if the id is not in the
// registry. It goes through Dashboards, so it honours hot-reload too.
func (r *Registry) ByID(id string) (Dashboard, bool) {
	for _, d := range r.Dashboards() {
		if d.ID == id {
			return d, true
		}
	}
	return Dashboard{}, false
}

// active is the registry the HTTP handlers serve from. It is installed once
// during startup by cmd/server/main.go (and by tests). A nil active registry
// serves no dashboards rather than panicking: GET /dashboards returns an
// empty list and GET /dashboards/{id} 404s, exactly as an empty registry did
// before this type existed.
var (
	activeMu sync.RWMutex
	active   *Registry
)

// SetActive installs the registry the handlers read from.
func SetActive(r *Registry) {
	activeMu.Lock()
	defer activeMu.Unlock()
	active = r
}

// Active returns the installed registry, which may be nil.
func Active() *Registry {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return active
}

// All returns every dashboard the active registry serves.
func All() []Dashboard { return Active().Dashboards() }

// ByID looks a dashboard up by id in the active registry.
func ByID(id string) (Dashboard, bool) { return Active().ByID(id) }

// LoadDir reads every *.json file in dir as one dashboard definition. The
// filename is not significant in any way: id, displayName, type and the rest
// all come from the file's content, and files are processed in lexical
// filename order purely so the resulting order (which is what the frontend's
// dashboard picker shows) is deterministic.
//
// Every failure is an error naming the offending file, and none of them is
// recoverable by skipping the file. A dropped dashboard is invisible: the
// picker simply has one fewer entry, with nothing anywhere saying why. That
// is the same failure class as a silently-ignored filter, which this project
// has been bitten by repeatedly, so an unreadable file, a malformed one, a
// missing id, a duplicate id, a bad type and a contradictory type/isTeamBased
// combination all fail the whole load.
//
// An empty directory is legal and yields no dashboards: a deployment that has
// not authored any definitions yet must still start and serve every other
// endpoint. A missing directory is not legal — it is a misconfigured path.
func LoadDir(dir string) ([]Dashboard, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("dashboard definitions: read directory %q: %w", dir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), definitionExt) {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	loaded := make([]sourced, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path) //nolint:gosec // path is deployment configuration, not user input
		if err != nil {
			return nil, fmt.Errorf("dashboard definitions: read %q: %w", path, err)
		}
		var d Dashboard
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, fmt.Errorf("dashboard definitions: parse %q: %w", path, err)
		}
		loaded = append(loaded, sourced{dashboard: d, source: path})
	}

	return finalize(loaded, true)
}

// finalize runs the shared post-decode pipeline over a decoded set: the
// deprecated-key migration first (so validation sees the current shape), then
// cross-field validation. requireType is true for the directory loader, where
// every definition is authored against the current schema, and false for the
// deprecated DASHBOARDS_CONFIG path, whose already-deployed values predate
// the type field entirely.
func finalize(loaded []sourced, requireType bool) ([]Dashboard, error) {
	dashboards := make([]Dashboard, 0, len(loaded))
	for _, l := range loaded {
		dashboards = append(dashboards, l.dashboard)
	}
	migrateLegacyWidgetKeys(dashboards)
	for i := range loaded {
		loaded[i].dashboard = dashboards[i]
	}

	if err := validate(loaded, requireType); err != nil {
		return nil, err
	}
	return dashboards, nil
}

// validate enforces every rule that cannot be expressed in the JSON shape.
//
// The type / isDefault / isTeamBased trio is deliberately three independent
// fields rather than two derived from one, which means they can be set to
// states that contradict each other. Those states are rejected here, loudly
// and by filename, rather than normalised silently — a dashboard that quietly
// behaves as something other than what its file says is worse than a failed
// deploy:
//
//   - type cre or sre with isTeamBased false. Both are team-scoped by
//     definition: the frontend auto-selects them from the caller's own team
//     and offers a team picker. Without isTeamBased there is no picker, so
//     the dashboard can never be scoped to the team it claims to target.
//   - type cs with isTeamBased true. cs is the organisation-wide dashboard,
//     and it is what a caller with no team at all falls back to. A team
//     picker on it contradicts both roles.
//   - two isDefault dashboards sharing one type. Selection asks for "the
//     default dashboard of type X"; two answers means the one you get depends
//     on file ordering.
func validate(loaded []sourced, requireType bool) error {
	byID := make(map[string]string, len(loaded))
	defaultByType := make(map[Type]string, len(loaded))

	for _, l := range loaded {
		d := l.dashboard

		if strings.TrimSpace(d.ID) == "" {
			return fmt.Errorf("dashboard definitions: %s: \"id\" is empty; the id is the dashboard's identity and is never derived from the filename", l.source)
		}
		if prev, dup := byID[d.ID]; dup {
			return fmt.Errorf("dashboard definitions: %s: duplicate dashboard id %q, already defined by %s", l.source, d.ID, prev)
		}
		byID[d.ID] = l.source

		if strings.TrimSpace(d.DisplayName) == "" {
			return fmt.Errorf("dashboard definitions: %s (id %q): \"displayName\" is empty", l.source, d.ID)
		}

		if d.Type == "" {
			if requireType {
				return fmt.Errorf("dashboard definitions: %s (id %q): \"type\" is required; expected one of %q, %q, %q",
					l.source, d.ID, TypeCRE, TypeSRE, TypeCS)
			}
			slog.Warn("dashboard definitions: no \"type\" set; automatic dashboard selection cannot classify this dashboard",
				"source", l.source, "dashboardId", d.ID)
			continue
		}
		if !validTypes[d.Type] {
			return fmt.Errorf("dashboard definitions: %s (id %q): unknown \"type\" %q; expected one of %q, %q, %q",
				l.source, d.ID, d.Type, TypeCRE, TypeSRE, TypeCS)
		}

		switch {
		case (d.Type == TypeCRE || d.Type == TypeSRE) && !d.IsTeamBased:
			return fmt.Errorf("dashboard definitions: %s (id %q): contradictory configuration: \"type\": %q is team-scoped but \"isTeamBased\" is false; set isTeamBased true or change the type to %q",
				l.source, d.ID, d.Type, TypeCS)
		case d.Type == TypeCS && d.IsTeamBased:
			return fmt.Errorf("dashboard definitions: %s (id %q): contradictory configuration: \"type\": %q is organisation-wide but \"isTeamBased\" is true; set isTeamBased false or change the type to %q or %q",
				l.source, d.ID, d.Type, TypeCRE, TypeSRE)
		}

		if d.IsDefault {
			if prev, dup := defaultByType[d.Type]; dup {
				return fmt.Errorf("dashboard definitions: %s (id %q): a second \"isDefault\" dashboard of type %q; %s already claims it, and automatic selection needs exactly one",
					l.source, d.ID, d.Type, prev)
			}
			defaultByType[d.Type] = l.source
		}
	}

	return nil
}
