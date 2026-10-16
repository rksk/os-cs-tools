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

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

var globalSearchTrimCases = []struct {
	name string
	in   string
	want string
}{
	{"leading space", " CS0000001", "CS0000001"},
	{"trailing space", "CS0000001 ", "CS0000001"},
	{"tabs and newlines", "\t\nCS0000001\r\n\t", "CS0000001"},
	{"all whitespace is no query", " \t\n ", ""},
	{"empty", "", ""},
	{"normal query", "CS0000001", "CS0000001"},
	{"inner whitespace kept", "  jane  doe ", "jane  doe"},
}

func TestGlobalSearch_TrimsQuery(t *testing.T) {
	for _, tc := range globalSearchTrimCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeSearchRepo{}
			resp, err := newGlobal(customerScope, repo).GlobalSearch(context.Background(), domain.GlobalSearchRequest{
				Filters: &domain.GlobalSearchFilters{SearchQuery: tc.in},
			})
			if err != nil {
				t.Fatal(err)
			}
			if repo.projects == nil || repo.cases == nil {
				t.Fatal("search did not run")
			}
			if repo.projects.query != tc.want || repo.cases.query != tc.want {
				t.Errorf("repo queries = %q/%q, want %q", repo.projects.query, repo.cases.query, tc.want)
			}
			if resp.Query != tc.want {
				t.Errorf("resp.Query = %q, want %q", resp.Query, tc.want)
			}
		})
	}
}

// The 200-character limit applies to the trimmed value, so padding does not
// push an otherwise valid query over it.
func TestGlobalSearch_LengthLimitAppliesToTrimmedQuery(t *testing.T) {
	repo := &fakeSearchRepo{}
	q := strings.Repeat("x", 200)
	if _, err := newGlobal(customerScope, repo).GlobalSearch(context.Background(), domain.GlobalSearchRequest{
		Filters: &domain.GlobalSearchFilters{SearchQuery: "  " + q + "  "},
	}); err != nil {
		t.Fatalf("padded 200-char query rejected: %v", err)
	}
}

func TestSNGlobalSearch_TrimsQuery(t *testing.T) {
	for _, tc := range globalSearchTrimCases {
		t.Run(tc.name, func(t *testing.T) {
			var sent snGlobalSearchPayload
			mux := http.NewServeMux()
			mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(body, &sent); err != nil {
					t.Errorf("bad payload: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				// Echo the raw query as an untrimmed upstream would.
				_, _ = w.Write([]byte(`{"query":` + mustJSON(tc.in) + `,"projects":[],"cases":[]}`))
			})
			svc := NewServiceNowGlobalService(newTestSNClient(t, mux))

			resp, err := svc.GlobalSearch(contextWithUserIDToken("token"), domain.GlobalSearchRequest{
				Filters: &domain.GlobalSearchFilters{SearchQuery: tc.in},
			})
			if err != nil {
				t.Fatal(err)
			}
			if sent.Filters == nil || sent.Filters.SearchQuery != tc.want {
				t.Errorf("outbound searchQuery = %+v, want %q", sent.Filters, tc.want)
			}
			if resp.Query != tc.want {
				t.Errorf("resp.Query = %q, want %q", resp.Query, tc.want)
			}
		})
	}
}

func mustJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
