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
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// parseIncidentFilters runs the Postgres incident filter parser over the given
// field filters, the way a search request reaches it.
func parseIncidentFilters(filters ...domain.IncidentFieldFilter) (states, serviceIDs []string, madeSla, slaViolated *bool, err error) {
	_, states, serviceIDs, _, madeSla, slaViolated, _, _, err = parseIncidentFieldFiltersPostgres(
		domain.SearchIncidentsFilters{Filters: filters}, time.Now().UTC())
	return states, serviceIDs, madeSla, slaViolated, err
}

func stateKeys(values ...string) domain.IncidentFieldFilter {
	return domain.IncidentFieldFilter{Field: "incidentStateKeys", Op: "in", Values: values}
}

func stateIn(values ...string) domain.IncidentFieldFilter {
	return domain.IncidentFieldFilter{Field: "state", Op: "in", Values: values}
}

// An "incidentStateKeys" filter used to be accepted and then ignored on this
// data source, so a request for the open incidents of a service answered with
// every incident the service ever had.
func TestParseIncidentFieldFiltersPostgres_IncidentStateKeysAreApplied(t *testing.T) {
	states, _, _, _, err := parseIncidentFilters(stateKeys("1", "2"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"NEW", "IN_PROGRESS"}; !reflect.DeepEqual(states, want) {
		t.Fatalf("states = %v, want %v (a dropped filter would leave this empty and match every incident)", states, want)
	}
}

func TestParseIncidentFieldFiltersPostgres_EveryServiceNowKeyMapsToItsEnumLabel(t *testing.T) {
	// ServiceNow's incident_state keys, and the incident_state_enum label each
	// one is stored under. CANCELED has one L in the enum, two in the domain.
	cases := map[string]string{
		"1": "NEW",
		"2": "IN_PROGRESS",
		"3": "ON_HOLD",
		"6": "RESOLVED",
		"7": "CLOSED",
		"8": "CANCELED",
	}
	for key, label := range cases {
		states, _, _, _, err := parseIncidentFilters(stateKeys(key))
		if err != nil {
			t.Fatalf("key %s: unexpected error: %v", key, err)
		}
		if want := []string{label}; !reflect.DeepEqual(states, want) {
			t.Errorf("key %s: states = %v, want %v", key, states, want)
		}
	}
}

func TestParseIncidentFieldFiltersPostgres_StateKeysKeepOrderAndDropRepeats(t *testing.T) {
	states, _, _, _, err := parseIncidentFilters(stateKeys("2", "1", "2"), stateKeys("1", "6"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"IN_PROGRESS", "NEW", "RESOLVED"}; !reflect.DeepEqual(states, want) {
		t.Fatalf("states = %v, want %v", states, want)
	}
}

func TestParseIncidentFieldFiltersPostgres_StateKeyWithNoStateIsRejectedNotSkipped(t *testing.T) {
	// 4, 5 and 9 are valid ServiceNow numbers with no state of ours. Skipping
	// one would narrow nothing, and a list left empty means "no state filter".
	for _, key := range []string{"4", "5", "9", "99"} {
		_, _, _, _, err := parseIncidentFilters(stateKeys("1", key))
		var ve *apierror.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("key %s: want a validation error, got %v", key, err)
		}
		if !strings.Contains(ve.Msg, `"`+key+`"`) || !strings.Contains(ve.Msg, "no equivalent incident state") {
			t.Errorf("key %s: message %q should name the value and say why", key, ve.Msg)
		}
	}
}

func TestParseIncidentFieldFiltersPostgres_StateKeysRejections(t *testing.T) {
	tests := []struct {
		name   string
		filter domain.IncidentFieldFilter
	}{
		{"not a number", stateKeys("two")},
		{"negative", stateKeys("-1")},
		{"empty list", stateKeys()},
		{"wrong op", domain.IncidentFieldFilter{Field: "incidentStateKeys", Op: "eq", Values: []string{"1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, _, err := parseIncidentFilters(tt.filter)
			var ve *apierror.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want a validation error, got %v", err)
			}
		})
	}
}

// "state" and "incidentStateKeys" both constrain the one column here, so an
// incident has to satisfy both, as the two filters combine on ServiceNow.
func TestParseIncidentFieldFiltersPostgres_StateAndStateKeysAreAnded(t *testing.T) {
	states, _, _, _, err := parseIncidentFilters(stateIn("NEW", "IN_PROGRESS", "ON_HOLD"), stateKeys("3", "2", "6"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"IN_PROGRESS", "ON_HOLD"}; !reflect.DeepEqual(states, want) {
		t.Fatalf("states = %v, want the two the filters share, %v", states, want)
	}
}

func TestParseIncidentFieldFiltersPostgres_StatesNoIncidentCanSatisfyAreRejected(t *testing.T) {
	// An empty list would read as "no state filter" and answer with everything:
	// the opposite of "nothing can match".
	_, _, _, _, err := parseIncidentFilters(stateIn("NEW"), stateKeys("2"))
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want a validation error, got %v", err)
	}
	if !strings.Contains(ve.Msg, "no state in common") {
		t.Errorf("message %q should say the two filters share no state", ve.Msg)
	}
}

func TestParseIncidentFieldFiltersPostgres_StateFilterAloneIsUnchanged(t *testing.T) {
	states, _, _, _, err := parseIncidentFilters(stateIn("CANCELLED", "NEW"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"CANCELED", "NEW"}; !reflect.DeepEqual(states, want) {
		t.Fatalf("states = %v, want %v", states, want)
	}
}

// The widget that counts open incidents with a breached SLA: the open states,
// the reliable breach signal, and the services it covers.
func TestParseIncidentFieldFiltersPostgres_OpenIncidentsWithABreachedSLA(t *testing.T) {
	service := "11111111-2222-4333-8444-555555555555"
	states, serviceIDs, madeSla, slaViolated, err := parseIncidentFilters(
		stateIn("NEW", "IN_PROGRESS"),
		domain.IncidentFieldFilter{Field: "slaViolated", Op: "eq", Values: []string{"true"}},
		domain.IncidentFieldFilter{Field: "businessServiceId", Op: "in", Values: []string{service}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"NEW", "IN_PROGRESS"}; !reflect.DeepEqual(states, want) {
		t.Errorf("states = %v, want %v", states, want)
	}
	if want := []string{service}; !reflect.DeepEqual(serviceIDs, want) {
		t.Errorf("serviceIDs = %v, want %v", serviceIDs, want)
	}
	if slaViolated == nil || !*slaViolated {
		t.Errorf("slaViolated = %v, want pointer to true", slaViolated)
	}
	if madeSla != nil {
		t.Errorf("madeSla = %v, want nil: that field must not be needed", madeSla)
	}
}

// The definition a ServiceNow-parity dashboard carries: with the state keys
// applied it counts only the open incidents, not every incident of the services.
func TestParseIncidentFieldFiltersPostgres_ServiceNowParityDefinitionIsRestrictedToOpenIncidents(t *testing.T) {
	states, _, madeSla, _, err := parseIncidentFilters(
		stateKeys("1", "2"),
		domain.IncidentFieldFilter{Field: "madeSla", Op: "eq", Values: []string{"true"}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"NEW", "IN_PROGRESS"}; !reflect.DeepEqual(states, want) {
		t.Errorf("states = %v, want %v", states, want)
	}
	if madeSla == nil || !*madeSla {
		t.Errorf("madeSla = %v, want pointer to true", madeSla)
	}
}

func TestIncidentStateBySNKeyIsTheInverseOfTheServiceNowKeyMap(t *testing.T) {
	if len(incidentStateBySNKey) != len(snIncidentStateKeyMap) {
		t.Fatalf("%d keys read back for %d states", len(incidentStateBySNKey), len(snIncidentStateKeyMap))
	}
	for state, key := range snIncidentStateKeyMap {
		if got := incidentStateBySNKey[key]; got != state {
			t.Errorf("key %d reads back as %q, want %q", key, got, state)
		}
	}
}

// capturingIncidentSearchRepo records the state list a search or an aggregation
// reaches the repository with.
type capturingIncidentSearchRepo struct {
	*stubIncidentRepo
	states []string
	calls  int
}

func (c *capturingIncidentSearchRepo) SearchIncidents(_ context.Context, _ domain.SearchIncidentsRequest, _, states, _, _ []string, _, _ *bool, _, _ *time.Time) ([]domain.SearchIncidentView, int, error) {
	c.states = states
	c.calls++
	return nil, 0, nil
}

func (c *capturingIncidentSearchRepo) AggregateIncidents(_ context.Context, _ domain.SearchIncidentsRequest, _, states, _, _ []string, _, _ *bool, _, _ *time.Time, _ string, _ int) (domain.AggregateResponse, error) {
	c.states = states
	c.calls++
	return domain.AggregateResponse{}, nil
}

func TestIncidentService_SearchAndAggregateHandTheMappedStatesToTheRepository(t *testing.T) {
	filters := domain.SearchIncidentsFilters{Filters: []domain.IncidentFieldFilter{stateKeys("1", "2")}}
	want := []string{"NEW", "IN_PROGRESS"}

	repo := &capturingIncidentSearchRepo{stubIncidentRepo: &stubIncidentRepo{}}
	svc := NewIncidentService(repo, nil)

	if _, err := svc.SearchIncidents(context.Background(), domain.SearchIncidentsRequest{Filters: filters}); err != nil {
		t.Fatalf("SearchIncidents: %v", err)
	}
	if !reflect.DeepEqual(repo.states, want) {
		t.Errorf("SearchIncidents reached the repository with states %v, want %v", repo.states, want)
	}

	repo.states = nil
	if _, err := svc.AggregateIncidents(context.Background(), domain.AggregateIncidentsRequest{Filters: filters, GroupBy: "state"}); err != nil {
		t.Fatalf("AggregateIncidents: %v", err)
	}
	if !reflect.DeepEqual(repo.states, want) {
		t.Errorf("AggregateIncidents reached the repository with states %v, want %v", repo.states, want)
	}
	if repo.calls != 2 {
		t.Errorf("repository called %d times, want 2", repo.calls)
	}
}

func TestIncidentService_ARejectedStateKeyNeverReachesTheRepository(t *testing.T) {
	repo := &capturingIncidentSearchRepo{stubIncidentRepo: &stubIncidentRepo{}}
	svc := NewIncidentService(repo, nil)

	_, err := svc.SearchIncidents(context.Background(), domain.SearchIncidentsRequest{
		Filters: domain.SearchIncidentsFilters{Filters: []domain.IncidentFieldFilter{stateKeys("1", "5")}},
	})

	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want a validation error, got %v", err)
	}
	if repo.calls != 0 {
		t.Errorf("the repository was called %d times for a request that is refused", repo.calls)
	}
}
