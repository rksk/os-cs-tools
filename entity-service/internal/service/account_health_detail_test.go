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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

type healthDetailRepo struct {
	repository.AccountRepository
	row repository.AccountHealthDetailRow
	err error
	got string
}

func (r *healthDetailRepo) GetAccountHealthDetail(_ context.Context, id string) (repository.AccountHealthDetailRow, error) {
	r.got = id
	return r.row, r.err
}

const healthDetailAccountID = "00000000-0000-0000-0000-000000000001"

func TestGetAccountHealthDetail_RejectsMalformedID(t *testing.T) {
	repo := &healthDetailRepo{}
	_, err := NewAccountService(repo).GetAccountHealthDetail(context.Background(), "not-a-uuid")
	var verr *apierror.ValidationError
	if !errors.As(err, &verr) || repo.got != "" {
		t.Fatalf("err = %v (repo called with %q), want ValidationError before any query", err, repo.got)
	}
}

func TestGetAccountHealthDetail_PassesNotFoundThrough(t *testing.T) {
	repo := &healthDetailRepo{err: &apierror.NotFoundError{Msg: "account not found"}}
	_, err := NewAccountService(repo).GetAccountHealthDetail(context.Background(), healthDetailAccountID)
	var nf *apierror.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}

func TestGetAccountHealthDetail_BuildsGoLiveStatusPerProject(t *testing.T) {
	activation := time.Now().AddDate(0, 0, -100)
	months := 3
	repo := &healthDetailRepo{row: repository.AccountHealthDetailRow{
		AccountName:    "Acme",
		ActivationDate: &activation,
		GoLiveMonths:   &months,
		Projects: []repository.ProjectHealthDetailRow{
			{GoLiveState: "pending", Detail: domain.ProjectHealthDetail{ID: "p1", Name: "One"}},
			{GoLiveState: "success", Detail: domain.ProjectHealthDetail{ID: "p2", Name: "Two"}},
			{GoLiveState: "failure", Detail: domain.ProjectHealthDetail{ID: "p3", Name: "Three"}},
		},
	}}
	got, err := NewAccountService(repo).GetAccountHealthDetail(context.Background(), healthDetailAccountID)
	if err != nil {
		t.Fatal(err)
	}
	if repo.got != healthDetailAccountID || got.AccountName != "Acme" || len(got.CustomerProjects) != 3 {
		t.Fatalf("got %+v (repo id %q)", got, repo.got)
	}
	want := []domain.AccountGoLiveStatus{
		{Status: "Hasn't Gone Live Since 3 months", State: "pending"},
		{Status: "Has Gone Live", State: "success"},
		{Status: "Hasn't Gone Live in 6 months", IsRisk: true, State: "failure"},
	}
	for i, w := range want {
		if g := got.CustomerProjects[i].GoLiveStatus; g != w {
			t.Errorf("project %d: %+v, want %+v", i, g, w)
		}
		if got.CustomerProjects[i].ID != repo.row.Projects[i].Detail.ID {
			t.Errorf("project %d out of order", i)
		}
	}
}

func TestGetAccountHealthDetail_NoProjectsIsAnEmptyArray(t *testing.T) {
	repo := &healthDetailRepo{row: repository.AccountHealthDetailRow{AccountName: "Bare"}}
	got, err := NewAccountService(repo).GetAccountHealthDetail(context.Background(), healthDetailAccountID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), `"customerProjects":[]`) {
		t.Errorf("customerProjects must be an empty array, got %s", b)
	}
}

func TestSNAccountService_GetAccountHealthDetail_IsRejected(t *testing.T) {
	_, err := NewServiceNowAccountService(nil).GetAccountHealthDetail(context.Background(), healthDetailAccountID)
	var verr *apierror.ValidationError
	if !errors.As(err, &verr) || !strings.Contains(verr.Msg, "DATA_SOURCE=servicenow") {
		t.Fatalf("err = %v, want a ValidationError naming the data source", err)
	}
}
