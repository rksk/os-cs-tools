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

import "testing"

func TestAbsHours(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		name string
		in   *float64
		want *float64
	}{
		{"negative becomes positive", f(-14), f(14)},
		{"negative fraction", f(-12.5), f(12.5)},
		{"positive unchanged", f(14), f(14)},
		{"zero unchanged", f(0), f(0)},
		{"nil stays nil", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := absHours(c.in)
			if (got == nil) != (c.want == nil) {
				t.Fatalf("absHours(%v) = %v, want %v", c.in, got, c.want)
			}
			if got != nil && *got != *c.want {
				t.Fatalf("absHours(%v) = %v, want %v", *c.in, *got, *c.want)
			}
		})
	}
}
