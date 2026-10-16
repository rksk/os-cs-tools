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

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

type healthDetailAccountSvc struct {
	service.AccountService
	detail domain.AccountHealthDetail
	err    error
	gotID  string
}

func (s *healthDetailAccountSvc) GetAccountHealthDetail(_ context.Context, id string) (domain.AccountHealthDetail, error) {
	s.gotID = id
	return s.detail, s.err
}

func serveHealthDetail(t *testing.T, svc *healthDetailAccountSvc, id string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /accounts/{id}/health-detail", NewAccountHandler(svc).GetAccountHealthDetail)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/accounts/"+id+"/health-detail", nil))
	return rec
}

func TestGetAccountHealthDetail_Handler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		svc := &healthDetailAccountSvc{detail: domain.AccountHealthDetail{AccountName: "Acme", CustomerProjects: []domain.ProjectHealthDetail{}}}
		rec := serveHealthDetail(t, svc, "abc")
		if rec.Code != http.StatusOK || svc.gotID != "abc" {
			t.Fatalf("status %d, id %q", rec.Code, svc.gotID)
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got["accountName"] != "Acme" {
			t.Fatalf("body %s (%v)", rec.Body.String(), err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		rec := serveHealthDetail(t, &healthDetailAccountSvc{err: &apierror.NotFoundError{Msg: "account not found"}}, "abc")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status %d, want 404", rec.Code)
		}
	})
	t.Run("validation", func(t *testing.T) {
		rec := serveHealthDetail(t, &healthDetailAccountSvc{err: &apierror.ValidationError{Msg: "bad"}}, "abc")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status %d, want 400", rec.Code)
		}
	})
}
