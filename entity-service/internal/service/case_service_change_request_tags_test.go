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
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// A change request's tags are Postgres-only: POST /cases/{id}/tags and
// DELETE /cases/{id}/tags/{tagId} take any work item id, and in dual-write
// mode the case path mirrors a tag change to the previous system's case tag
// endpoint -- which only a case-like work item has a record behind. The
// mirror is gated on CaseRepository.IsCaseLikeWorkItem: for a change request
// nothing is dispatched (no mirror call, no sn_writeback_failures row, no
// sys_id lookup), and the Postgres write is the whole operation.

// notCaseLike is a stub repository whose work item is a change request.
func notCaseLikeTagRepo(t *testing.T) (*stubCaseRepo, *int) {
	t.Helper()
	sysIDLookups := 0
	repo := &stubCaseRepo{
		addCaseTag: func(_ context.Context, _ string, label, _ string) (domain.Tag, error) {
			return domain.Tag{ID: "t-1", Label: label}, nil
		},
		removeCaseTag: func(context.Context, string, string, string) error { return nil },
		getCaseTagSNSysID: func(context.Context, string, string) (*string, error) {
			sysIDLookups++
			stored := uuidToSysid(testSNSysID2)
			return &stored, nil
		},
		isCaseLikeWorkItem: func(_ context.Context, id string) (bool, error) {
			if id != testDeploymentUUID {
				t.Errorf("IsCaseLikeWorkItem asked about %q, want %q", id, testDeploymentUUID)
			}
			return false, nil
		},
	}
	return repo, &sysIDLookups
}

func dualWriteTagService(repo repository.CaseRepository, mirror CaseService, failures *recordingSNWritebackFailures) CaseService {
	userRepo := stubUserRepo{getUserByEmail: func(context.Context, string) (domain.User, error) {
		return domain.User{ID: testDeploymentUUID, Email: "jane.doe@example.com"}, nil
	}}
	return NewCaseServiceWithSNWriteback(repo, userRepo, nil, alwaysUnrestrictedAccess{}, nil, NewSNWritebackDispatcher(failures), mirror, nil, "")
}

func TestCaseService_AddCaseTag_ChangeRequestIsNotMirrored(t *testing.T) {
	repo, _ := notCaseLikeTagRepo(t)
	mirrorCalled := make(chan struct{}, 1)
	mirror := &stubMirrorCaseService{
		addCaseTagAsFn: func(context.Context, string, string, string) (domain.Tag, error) {
			mirrorCalled <- struct{}{}
			return domain.Tag{}, nil
		},
	}
	failures := &recordingSNWritebackFailures{}
	svc := dualWriteTagService(repo, mirror, failures)

	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))
	tag, err := svc.AddCaseTag(ctx, testDeploymentUUID, "maintenance-window")
	if err != nil {
		t.Fatalf("AddCaseTag on a change request: %v", err)
	}
	if tag.Label != "maintenance-window" {
		t.Errorf("tag = %+v, want the Postgres tag back", tag)
	}

	select {
	case <-mirrorCalled:
		t.Fatal("mirror.AddCaseTagAs was called for a change request's tag")
	case <-time.After(300 * time.Millisecond):
		// expected: a change request's tags are Postgres-only
	}
	if got := failures.count(); got != 0 {
		t.Errorf("expected 0 sn_writeback_failures records (nothing dispatched), got %d", got)
	}
}

func TestCaseService_RemoveCaseTag_ChangeRequestIsNotMirrored(t *testing.T) {
	repo, sysIDLookups := notCaseLikeTagRepo(t)
	mirrorCalled := make(chan struct{}, 1)
	mirror := &stubMirrorCaseService{
		removeCaseTagFn: func(context.Context, string, string) error {
			mirrorCalled <- struct{}{}
			return nil
		},
	}
	failures := &recordingSNWritebackFailures{}
	svc := dualWriteTagService(repo, mirror, failures)

	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))
	if err := svc.RemoveCaseTag(ctx, testDeploymentUUID, testUUID); err != nil {
		t.Fatalf("RemoveCaseTag on a change request: %v", err)
	}

	select {
	case <-mirrorCalled:
		t.Fatal("mirror.RemoveCaseTag was called for a change request's tag")
	case <-time.After(300 * time.Millisecond):
		// expected
	}
	if *sysIDLookups != 0 {
		t.Errorf("GetCaseTagSNSysID was called %d times for a change request; the mirror it feeds does not apply", *sysIDLookups)
	}
	if got := failures.count(); got != 0 {
		t.Errorf("expected 0 sn_writeback_failures records, got %d", got)
	}
}

// The gate fails closed: when the work item's type cannot be read, the tag
// change is still made in Postgres but nothing is mirrored, rather than a
// mirror against a record that may not be a case.
func TestCaseService_AddCaseTag_TypeLookupFailureSkipsTheMirror(t *testing.T) {
	repo, _ := notCaseLikeTagRepo(t)
	repo.isCaseLikeWorkItem = func(context.Context, string) (bool, error) {
		return false, errors.New("connection reset")
	}
	mirrorCalled := make(chan struct{}, 1)
	mirror := &stubMirrorCaseService{
		addCaseTagAsFn: func(context.Context, string, string, string) (domain.Tag, error) {
			mirrorCalled <- struct{}{}
			return domain.Tag{}, nil
		},
	}
	failures := &recordingSNWritebackFailures{}
	svc := dualWriteTagService(repo, mirror, failures)

	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))
	if _, err := svc.AddCaseTag(ctx, testDeploymentUUID, "patch"); err != nil {
		t.Fatalf("the Postgres attach must still succeed: %v", err)
	}
	select {
	case <-mirrorCalled:
		t.Fatal("mirror.AddCaseTagAs was called although the work item's type is unknown")
	case <-time.After(300 * time.Millisecond):
	}
	if got := failures.count(); got != 0 {
		t.Errorf("expected 0 sn_writeback_failures records, got %d", got)
	}
}

// And a case-like work item keeps the mirror it always had (the gate answers
// true): TestCaseService_AddCaseTag_MirrorsToServiceNow and the RemoveCaseTag
// mirror tests in case_service_test.go run with the stub's default answer.
// This pins that the default is really "case-like", so those tests mean
// what they say.
func TestStubCaseRepo_IsCaseLikeWorkItemDefaultsToTrue(t *testing.T) {
	caseLike, err := (&stubCaseRepo{}).IsCaseLikeWorkItem(context.Background(), testDeploymentUUID)
	if err != nil || !caseLike {
		t.Fatalf("stub default = %v, %v; want true, nil", caseLike, err)
	}
}
