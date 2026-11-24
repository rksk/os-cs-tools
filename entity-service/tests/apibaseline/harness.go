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

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Actor is one caller identity, expressed the way the gateway forwards it:
// an x-jwt-assertion (client) and/or an x-user-id-token (user).
type Actor struct {
	Name   string
	Client string // client id for x-jwt-assertion, "" for none
	Email  string // user email for x-user-id-token, "" for none
	Roles  []string
	Raw    map[string]string // raw header overrides (used for malformed-token cases)
}

// Resp is one HTTP response, with the recorded result it produced.
type Resp struct {
	Status int
	Raw    []byte
	JSON   any
	c      *Ctx
	idx    int // index into c.results, -1 when not recorded
}

// Result is what is written to the results file for one recorded call.
type Result struct {
	Flow    string `json:"flow"`
	Step    string `json:"step"`
	Call    int    `json:"call"`
	Expect  string `json:"expectedOnMain"`
	Request struct {
		Actor  string `json:"actor"`
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   any    `json:"body,omitempty"`
	} `json:"request"`
	Status      int    `json:"status"`
	ContentType string `json:"contentType,omitempty"`
	Body        any    `json:"body,omitempty"`
	WantStatus  int    `json:"wantStatus,omitempty"`
	AsExpected  *bool  `json:"asExpected,omitempty"`
}

// Ctx carries run-wide state for the scenarios.
type Ctx struct {
	base    string
	s       *signer
	hc      *http.Client
	db      string
	psql    string
	aliases map[string]string
	vars    map[string]string
	results []Result
	cur     *Step
	calls   int
}

// Step is one named scenario step.
type Step struct {
	Flow   string
	Name   string
	Expect string // one-line statement of what main is expected to do
	Fn     func(c *Ctx)
}

var steps []Step

// reg registers a scenario step. Registration order is execution order:
// later steps may use ids captured by earlier ones.
func reg(flow, name, expect string, fn func(c *Ctx)) {
	steps = append(steps, Step{Flow: flow, Name: name, Expect: expect, Fn: fn})
}

// Do sends a request as actor and records it under the current step.
func (c *Ctx) Do(a Actor, method, path string, body any) *Resp {
	return c.do(a, method, path, body, true)
}

// Quiet sends a request but does not record it (fixture preparation).
func (c *Ctx) Quiet(a Actor, method, path string, body any) *Resp {
	return c.do(a, method, path, body, false)
}

func (c *Ctx) do(a Actor, method, path string, body any, record bool) *Resp {
	var rdr io.Reader
	var reqBody any
	if body != nil {
		if s, ok := body.(string); ok {
			rdr = strings.NewReader(s)
			reqBody = s
		} else {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
			reqBody = body
		}
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		panic(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.Raw != nil {
		for k, v := range a.Raw {
			req.Header.Set(k, v)
		}
	} else {
		if a.Client != "" {
			req.Header.Set("x-jwt-assertion", c.s.clientAssertion(a.Client))
		}
		if a.Email != "" {
			req.Header.Set("x-user-id-token", c.s.userToken(a.Email, a.Roles))
		}
	}
	resp, err := c.hc.Do(req)
	r := &Resp{c: c, idx: -1}
	var ct string
	if err != nil {
		r.Status = -1
		r.Raw = []byte(err.Error())
	} else {
		defer resp.Body.Close()
		r.Status = resp.StatusCode
		r.Raw, _ = io.ReadAll(resp.Body)
		ct = strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
		dec := json.NewDecoder(bytes.NewReader(r.Raw))
		dec.UseNumber()
		var v any
		if dec.Decode(&v) == nil {
			r.JSON = v
		}
	}
	if record && c.cur != nil {
		c.calls++
		res := Result{Flow: c.cur.Flow, Step: c.cur.Name, Call: c.calls, Expect: c.cur.Expect, Status: r.Status, ContentType: ct}
		res.Request.Actor, res.Request.Method = a.Name, method
		res.Request.Path = c.normString(path)
		if reqBody != nil {
			res.Request.Body = c.norm("", reqBody)
		}
		res.Body = c.normBody(r)
		c.results = append(c.results, res)
		r.idx = len(c.results) - 1
	}
	return r
}

// Expect records the status main is expected to answer with, and whether it did.
func (r *Resp) Expect(status int) *Resp {
	if r.idx >= 0 {
		ok := r.Status == status
		r.c.results[r.idx].WantStatus = status
		r.c.results[r.idx].AsExpected = &ok
	}
	return r
}

// Get walks a dotted path ("a.b.0.c") through the JSON body.
func (r *Resp) Get(path string) any {
	cur := r.JSON
	for _, p := range strings.Split(path, ".") {
		switch t := cur.(type) {
		case map[string]any:
			cur = t[p]
		case []any:
			var i int
			if _, err := fmt.Sscanf(p, "%d", &i); err != nil || i < 0 || i >= len(t) {
				return nil
			}
			cur = t[i]
		default:
			return nil
		}
	}
	return cur
}

// Str returns the string at path, or "".
func (r *Resp) Str(path string) string {
	switch v := r.Get(path).(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	}
	return ""
}

// Capture stores the string at path under a variable and registers it as an
// alias so every later response masks the real value with the alias name.
func (r *Resp) Capture(path, name string) string {
	v := r.Str(path)
	if v == "" {
		r.c.vars[name] = ""
		return ""
	}
	r.c.vars[name] = v
	r.c.aliases[v] = "<" + name + ">"
	if r.idx >= 0 { // show the new alias on the response that created it
		r.c.results[r.idx].Body = r.c.normBody(r)
	}
	return v
}

// V returns a captured variable.
func (c *Ctx) V(name string) string { return c.vars[name] }

// SQL runs a statement against the ephemeral database as the superuser and
// returns its single-line output. Used only to prepare time-dependent state
// (backdating) and to read state back.
func (c *Ctx) SQL(q string) string {
	out, err := exec.Command(c.psql, c.db, "-v", "ON_ERROR_STOP=1", "-tAq", "-c", q).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sql failed: %v: %s\n", err, out)
		return "ERR"
	}
	return strings.TrimSpace(string(out))
}

// SQLRecord runs a read-back query and records its output as a result, so
// persisted state is compared and not only status codes.
func (c *Ctx) SQLRecord(label, q string) string {
	out := c.SQL(q)
	if c.cur != nil {
		c.calls++
		res := Result{Flow: c.cur.Flow, Step: c.cur.Name, Call: c.calls, Expect: c.cur.Expect}
		res.Request.Actor, res.Request.Method, res.Request.Path = "db", "SQL", label
		res.Body = c.normString(out)
		c.results = append(c.results, res)
	}
	return out
}

func cmdRun(args []string) (int, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	base := fs.String("base-url", "", "server base URL")
	dir := fs.String("dir", "", "key directory")
	issuer := fs.String("issuer", "", "token issuer")
	db := fs.String("db", "", "superuser connection URL of the ephemeral database")
	psql := fs.String("psql", "psql", "psql binary")
	tree := fs.String("tree", "", "source tree under test (recorded only)")
	out := fs.String("out", "", "results file")
	only := fs.String("only", "", "run only steps whose flow or name contains this")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	s, err := loadSigner(*dir, *issuer)
	if err != nil {
		return 2, err
	}
	c := &Ctx{base: strings.TrimRight(*base, "/"), s: s, hc: &http.Client{Timeout: 60 * time.Second},
		db: *db, psql: *psql, aliases: map[string]string{}, vars: map[string]string{}}
	seedAliases(c)
	for i := range steps {
		st := &steps[i]
		if *only != "" && !strings.Contains(st.Flow+"/"+st.Name, *only) {
			continue
		}
		c.cur, c.calls = st, 0
		func() {
			defer func() {
				if r := recover(); r != nil {
					res := Result{Flow: st.Flow, Step: st.Name, Call: c.calls + 1, Expect: st.Expect, Status: -2, Body: fmt.Sprintf("suite panic: %v", r)}
					c.results = append(c.results, res)
				}
			}()
			st.Fn(c)
		}()
	}
	file := ResultsFile{Tree: *tree, Steps: len(steps), Results: c.results}
	b, _ := json.MarshalIndent(file, "", " ")
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		return 2, err
	}
	var mism, panics int
	names := map[string]bool{}
	for _, r := range c.results {
		names[r.Flow+"/"+r.Step] = true
		if r.AsExpected != nil && !*r.AsExpected {
			mism++
			fmt.Fprintf(os.Stderr, "  not as expected: %s / %s #%d: want %d got %d\n", r.Flow, r.Step, r.Call, r.WantStatus, r.Status)
		}
		if r.Status == -2 || r.Status == -1 {
			panics++
			fmt.Fprintf(os.Stderr, "  suite/transport error: %s / %s #%d: %v\n", r.Flow, r.Step, r.Call, r.Body)
		}
	}
	fmt.Fprintf(os.Stderr, "[apibaseline] steps=%d recorded_calls=%d not_as_expected=%d errors=%d -> %s\n",
		len(names), len(c.results), mism, panics, *out)
	if panics > 0 {
		return 1, nil
	}
	return 0, nil
}

// ResultsFile is the on-disk results format.
type ResultsFile struct {
	Tree    string   `json:"tree"`
	Steps   int      `json:"registeredSteps"`
	Results []Result `json:"results"`
}

func cmdCompare(args []string) (int, error) {
	if len(args) != 2 {
		return 2, fmt.Errorf("usage: compare <a.json> <b.json>")
	}
	load := func(p string) (ResultsFile, error) {
		var f ResultsFile
		b, err := os.ReadFile(p)
		if err != nil {
			return f, err
		}
		return f, json.Unmarshal(b, &f)
	}
	a, err := load(args[0])
	if err != nil {
		return 2, err
	}
	b, err := load(args[1])
	if err != nil {
		return 2, err
	}
	key := func(r Result) string { return fmt.Sprintf("%s | %s | #%d", r.Flow, r.Step, r.Call) }
	am, bm := map[string]Result{}, map[string]Result{}
	var order []string
	for _, r := range a.Results {
		am[key(r)] = r
		order = append(order, key(r))
	}
	for _, r := range b.Results {
		if _, ok := am[key(r)]; !ok {
			order = append(order, key(r))
		}
		bm[key(r)] = r
	}
	diffs := 0
	for _, k := range order {
		ra, oka := am[k]
		rb, okb := bm[k]
		switch {
		case !oka:
			diffs++
			fmt.Printf("DIFF %s\n  only in B (%s)\n", k, args[1])
		case !okb:
			diffs++
			fmt.Printf("DIFF %s\n  only in A (%s)\n", k, args[0])
		default:
			var lines []string
			if ra.Status != rb.Status {
				lines = append(lines, fmt.Sprintf("status: A=%d B=%d", ra.Status, rb.Status))
			}
			if ra.ContentType != rb.ContentType {
				lines = append(lines, fmt.Sprintf("content-type: A=%q B=%q", ra.ContentType, rb.ContentType))
			}
			lines = append(lines, diffValues("body", ra.Body, rb.Body)...)
			if len(lines) > 0 {
				diffs++
				fmt.Printf("DIFF %s\n  request: %s %s as %s", k, ra.Request.Method, ra.Request.Path, ra.Request.Actor)
				if ra.Request.Body != nil {
					bb, _ := json.Marshal(ra.Request.Body)
					fmt.Printf(" body=%s", bb)
				}
				fmt.Println()
				for _, l := range lines {
					fmt.Println("  " + l)
				}
			}
		}
	}
	fmt.Printf("\n%d of %d recorded calls differ (A=%s, B=%s)\n", diffs, len(order), args[0], args[1])
	if diffs > 0 {
		return 1, nil
	}
	return 0, nil
}

func trunc(v any) string {
	b, _ := json.Marshal(v)
	s := string(b)
	if len(s) > 220 {
		s = s[:220] + "..."
	}
	return s
}

// diffValues reports structural differences between two normalized JSON values.
func diffValues(path string, a, b any) []string {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: A=%s B=%s", path, trunc(a), trunc(b))}
		}
		keys := map[string]bool{}
		for k := range x {
			keys[k] = true
		}
		for k := range y {
			keys[k] = true
		}
		var ks []string
		for k := range keys {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		var out []string
		for _, k := range ks {
			xv, xok := x[k]
			yv, yok := y[k]
			switch {
			case !xok:
				out = append(out, fmt.Sprintf("%s.%s: only in B = %s", path, k, trunc(yv)))
			case !yok:
				out = append(out, fmt.Sprintf("%s.%s: only in A = %s", path, k, trunc(xv)))
			default:
				out = append(out, diffValues(path+"."+k, xv, yv)...)
			}
		}
		return out
	case []any:
		y, ok := b.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: A=%s B=%s", path, trunc(a), trunc(b))}
		}
		var out []string
		n := len(x)
		if len(y) > n {
			n = len(y)
		}
		if len(x) != len(y) {
			out = append(out, fmt.Sprintf("%s: length A=%d B=%d", path, len(x), len(y)))
		}
		for i := 0; i < n && i < len(x) && i < len(y); i++ {
			out = append(out, diffValues(fmt.Sprintf("%s[%d]", path, i), x[i], y[i])...)
		}
		return out
	default:
		ab, _ := json.Marshal(a)
		bb, _ := json.Marshal(b)
		if string(ab) != string(bb) {
			return []string{fmt.Sprintf("%s: A=%s B=%s", path, trunc(a), trunc(b))}
		}
	}
	return nil
}

// Only replaces the recorded body with a projection of it. Used where main's
// own ordering of tied rows is not deterministic (see the notes in the
// scenarios), so only the stable part is compared.
func (r *Resp) Only(project func(body any) any) *Resp {
	if r.idx >= 0 {
		r.c.results[r.idx].Body = project(r.c.results[r.idx].Body)
	}
	return r
}

// lookup walks a dotted path through a normalized body.
func lookup(v any, path string) any {
	for _, p := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[p]
	}
	return v
}

// Canon sorts the array at path (in the recorded body) by the given dotted keys,
// so rows the server returns in no defined order compare equal.
func (r *Resp) Canon(path string, keys ...string) *Resp {
	return r.Only(func(body any) any {
		m, ok := body.(map[string]any)
		if !ok {
			return body
		}
		arr, ok := m[path].([]any)
		if !ok {
			return body
		}
		sortRows(arr, keys)
		return body
	})
}

func sortRows(arr []any, keys []string) {
	key := func(row any) string {
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprint(lookup(row, k)))
		}
		return strings.Join(parts, "|")
	}
	sort.SliceStable(arr, func(i, j int) bool { return key(arr[i]) < key(arr[j]) })
}
