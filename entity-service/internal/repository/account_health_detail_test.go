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

package repository

import (
	"strings"
	"testing"
)

func TestHealthDetailPriority(t *testing.T) {
	str := func(s string) *string { return &s }
	for name, tc := range map[string]struct {
		in   *string
		want string
	}{
		"no severity row":   {nil, "Unspecified"},
		"unmapped label":    {str("S9"), "Unspecified"},
		"catastrophic":      {str("S0"), "catastrophic"},
		"critical":          {str("S1"), "critical"},
		"high":              {str("S2"), "high"},
		"medium":            {str("S3"), "medium"},
		"low":               {str("S4"), "low"},
		"empty label":       {str(""), "Unspecified"},
		"lower case is not": {str("s1"), "Unspecified"},
	} {
		if got := healthDetailPriority(tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

// The statements are fixed text; this pins the windows and type lists the
// health summary and the BFF's own flags use, so a drive-by edit to one is seen.
func TestAccountHealthDetailSQLWindowsAndTypes(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		want      []string
	}{
		{"recent cases", accountHealthDetailRecentCasesSQL, []string{
			"interval '180 days'", "{CASE,SERVICE_REQUEST,ENGAGEMENT,SECURITY_REPORT_ANALYSIS}", "ORDER BY wi.created_on DESC NULLS LAST, wi.id"}},
		{"migrations", accountHealthDetailMigrationsSQL, []string{
			"interval '180 days'", "interval '60 days'", "eng.type = 'MIGRATION'", "IS DISTINCT FROM 'CLOSED'", "ORDER BY wi.created_on DESC NULLS LAST, wi.id"}},
		{"escalations", accountHealthDetailEscalationsSQL, []string{
			"ANNOUNCEMENT", "ORDER BY ce.created_on DESC, ce.id"}},
		{"deployments", accountHealthDetailDeploymentsSQL, []string{"ORDER BY d.created_on DESC, d.id"}},
		{"eol", accountHealthDetailEolSQL, []string{
			"dp.active IS NULL OR dp.active = TRUE", "ORDER BY dp.created_on DESC, dp.id"}},
	} {
		for _, w := range tc.want {
			if !strings.Contains(tc.sql, w) {
				t.Errorf("%s: statement lacks %q:\n%s", tc.name, w, tc.sql)
			}
		}
		if strings.Count(tc.sql, "$1") == 0 {
			t.Errorf("%s: statement does not bind the account id", tc.name)
		}
	}
	if strings.Contains(accountHealthDetailRecentCasesSQL, "ANNOUNCEMENT") {
		t.Error("recent cases must not include announcements")
	}
}
