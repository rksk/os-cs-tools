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
	"encoding/json"
	"regexp"
	"strings"
)

var (
	reUUID   = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	reTS     = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?`)
	reDate   = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	reNumber = regexp.MustCompile(`\b[A-Z]{2,6}-?\d{5,}\b`)
	reToken  = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`)
	// Numeric fields derived from the clock or from elapsed time. Everything
	// else that is an integer is kept, so counts and totals are compared.
	reTimeKey      = regexp.MustCompile(`(?i)(elapsed|percent|remaining|timeleft|minutes|seconds|hours|days|age|duration|ttl|epoch|timestamp|latency)`)
	reDurationText = regexp.MustCompile(`^\d+ (Second|Minute|Hour|Day|Week|Month)s?( \d+ (Second|Minute|Hour|Day|Week|Month)s?)*$`)
	reSecret       = regexp.MustCompile(`(?i)^(token|secret|secretkey|apikey|consumptionsecret|password|signature)$`)
)

// seedAliases gives the fixed seed rows readable names in results.
func seedAliases(c *Ctx) {
	for id, name := range seedNames {
		c.aliases[id] = "<" + name + ">"
	}
}

func (c *Ctx) normBody(r *Resp) any {
	if r.JSON != nil {
		return c.norm("", r.JSON)
	}
	if len(r.Raw) == 0 {
		return nil
	}
	s := string(r.Raw)
	if len(s) > 400 {
		s = s[:400]
	}
	return c.normString(s)
}

func (c *Ctx) norm(key string, v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[c.normString(k)] = c.norm(k, x)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = c.norm(key, x)
		}
		return out
	case string:
		if reSecret.MatchString(key) && t != "" {
			return "<redacted>"
		}
		// Elapsed and remaining durations rendered as text ("1 Day 23 Hours")
		// move with the clock and round across unit boundaries.
		if reTimeKey.MatchString(key) && reDurationText.MatchString(t) {
			return "<duration>"
		}
		return c.normString(t)
	case json.Number:
		if reTimeKey.MatchString(key) {
			return "<num>"
		}
		if strings.ContainsAny(t.String(), ".eE") {
			return "<num>"
		}
		return t
	default:
		// Request bodies built from Go values (maps, slices, ints) go through JSON first.
		switch v.(type) {
		case nil, bool, float64, int, int64:
			if f, ok := v.(float64); ok {
				if reTimeKey.MatchString(key) || f != float64(int64(f)) {
					return "<num>"
				}
				return int64(f)
			}
			return v
		}
		b, err := json.Marshal(v)
		if err != nil {
			return v
		}
		var x any
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.UseNumber()
		if dec.Decode(&x) != nil {
			return v
		}
		return c.norm(key, x)
	}
}

func (c *Ctx) normString(s string) string {
	for real, alias := range c.aliases {
		if strings.Contains(s, real) {
			s = strings.ReplaceAll(s, real, alias)
		}
	}
	s = reToken.ReplaceAllString(s, "<token>")
	s = reUUID.ReplaceAllString(s, "<uuid>")
	s = reTS.ReplaceAllString(s, "<ts>")
	s = reDate.ReplaceAllString(s, "<date>")
	s = reNumber.ReplaceAllStringFunc(s, func(m string) string {
		i := 0
		for i < len(m) && (m[i] < '0' || m[i] > '9') {
			i++
		}
		return m[:i] + "<n>"
	})
	return s
}
