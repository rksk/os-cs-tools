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
	"context"
	"testing"
)

func TestHidesDeletedComments(t *testing.T) {
	base := context.Background()
	cases := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{"no identity fails closed", base, true},
		{"restricted customer", WithCallerIdentity(base, SearchScope{ViewerEmail: "jane.doe@example.com", ProjectIDs: []string{"00000000-0000-0000-0000-000000000000"}}), true},
		{"restricted caller with internal access (mixed identity)", WithCallerIdentity(base, SearchScope{ViewerEmail: "jane.doe@example.com", HasInternalAccess: true}), true},
		{"unrestricted staff through the customer portal", WithCallerIdentity(base, SearchScope{Unrestricted: true, ViewerEmail: "jane.doe@example.com", ViaCustomerPortal: true}), true},
		{"unrestricted internal portal caller", WithCallerIdentity(base, SearchScope{Unrestricted: true, ViewerEmail: "jane.doe@example.com", HasInternalAccess: true}), false},
		{"machine-to-machine caller", WithCallerIdentity(base, SearchScope{Unrestricted: true}), false},
		{"background job", WithSystemIdentity(base), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hidesDeletedComments(tc.ctx); got != tc.want {
				t.Fatalf("hidesDeletedComments = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDeletedCommentFilter(t *testing.T) {
	customer := WithCallerIdentity(context.Background(), SearchScope{ViewerEmail: "jane.doe@example.com"})
	internal := WithSystemIdentity(context.Background())

	if got, want := deletedCommentFilter(customer, "cc."), " AND cc.deleted_at IS NULL"; got != want {
		t.Fatalf("customer aliased filter = %q, want %q", got, want)
	}
	if got, want := deletedCommentFilter(customer, ""), " AND deleted_at IS NULL"; got != want {
		t.Fatalf("customer unaliased filter = %q, want %q", got, want)
	}
	if got := deletedCommentFilter(internal, "cc."); got != "" {
		t.Fatalf("internal caller must not be filtered, got %q", got)
	}
}
