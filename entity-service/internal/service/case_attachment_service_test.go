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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/sftpgo"
)

const testCaseID = "00000000-0000-0000-0000-0000000000c1"
const testAttachmentID = "00000000-0000-0000-0000-0000000000a1"
const testStorageKey = "/attachments/cases/00000000-0000-0000-0000-0000000000c1/00000000-0000-0000-0000-0000000000a1/diagnostics.log"
const testJWTAssertion = "fake-jwt-assertion"

func testDataURI(payload string) string {
	return "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte(payload))
}

func validCreateAttachmentRequest() domain.CreateAttachmentRequest {
	return domain.CreateAttachmentRequest{
		ReferenceID:   testCaseID,
		ReferenceType: domain.ReferenceTypeCase,
		Name:          "diagnostics.log",
		Type:          "text/plain",
		File:          testDataURI("hello world"),
	}
}

func actorUserRepo(t *testing.T) stubUserRepo {
	t.Helper()
	return stubUserRepo{
		getUserByEmail: func(_ context.Context, email string) (domain.User, error) {
			if email != "jane.doe@example.com" {
				t.Fatalf("unexpected email looked up: %s", email)
			}
			return domain.User{ID: "user-jane", Email: "jane.doe@example.com", FirstName: "Jane", LastName: "Doe"}, nil
		},
	}
}

// caseRepoWithProject returns a stubCaseRepo whose GetCaseByID reports a case
// with no linked project (ProjectDetails nil), the common case in tests that
// don't care about project-scoped storage-key namespacing.
func caseRepoWithProject(base *stubCaseRepo) *stubCaseRepo {
	if base.getCaseByID == nil {
		base.getCaseByID = func(_ context.Context, id string) (domain.CaseView, error) {
			return domain.CaseView{ID: id}, nil
		}
	}
	return base
}

// fakeSFTPGoClient is a test double for SFTPGoFileClient, recording calls and
// returning caller-configured results.
type fakeSFTPGoClient struct {
	mintToken  func(ctx context.Context, email, jwtAssertion string) (*sftpgo.Token, error)
	writeFile  func(ctx context.Context, accessToken, storageKey string, data []byte) error
	readFile   func(ctx context.Context, accessToken, storageKey string) ([]byte, string, error)
	removeFile func(ctx context.Context, accessToken, storageKey string) error

	writtenKeys  []string
	writtenBytes [][]byte
	removedKeys  []string
}

func (f *fakeSFTPGoClient) MintToken(ctx context.Context, email, jwtAssertion string) (*sftpgo.Token, error) {
	if f.mintToken != nil {
		return f.mintToken(ctx, email, jwtAssertion)
	}
	return &sftpgo.Token{AccessToken: "fake-access-token"}, nil
}

func (f *fakeSFTPGoClient) WriteFile(ctx context.Context, accessToken, storageKey string, data []byte) error {
	f.writtenKeys = append(f.writtenKeys, storageKey)
	f.writtenBytes = append(f.writtenBytes, data)
	if f.writeFile != nil {
		return f.writeFile(ctx, accessToken, storageKey, data)
	}
	return nil
}

func (f *fakeSFTPGoClient) ReadFile(ctx context.Context, accessToken, storageKey string) ([]byte, string, error) {
	if f.readFile != nil {
		return f.readFile(ctx, accessToken, storageKey)
	}
	return nil, "", &apierror.NotFoundError{Msg: "not found"}
}

func (f *fakeSFTPGoClient) RemoveFile(ctx context.Context, accessToken, storageKey string) error {
	f.removedKeys = append(f.removedKeys, storageKey)
	if f.removeFile != nil {
		return f.removeFile(ctx, accessToken, storageKey)
	}
	return nil
}

// attachmentCtx builds a context carrying both x-user-id-token and
// x-jwt-assertion, the pair every SFTPGo-backed attachment path requires.
func attachmentCtx(t *testing.T) context.Context {
	t.Helper()
	return contextWithUserIDTokenAndJWTAssertion(fakeJWTWithEmail(t, "jane.doe@example.com"), testJWTAssertion)
}

// TestCaseService_CreateCaseAttachment_Succeeds proves a well-formed request
// (a base64 data URI, matching the same contract snCaseService.CreateCaseAttachment
// already uses) is decoded, relayed to SFTPGo, and only then persisted as a
// 'complete' metadata row.
func TestCaseService_CreateCaseAttachment_Succeeds(t *testing.T) {
	var capturedReq domain.CreateAttachmentRequest
	createdOn := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	repo := caseRepoWithProject(&stubCaseRepo{
		createCaseAttachment: func(_ context.Context, req domain.CreateAttachmentRequest) (domain.Attachment, error) {
			capturedReq = req
			return domain.Attachment{
				ID:            testAttachmentID,
				ReferenceID:   req.ReferenceID,
				ReferenceType: domain.ReferenceTypeCase,
				Name:          req.Name,
				Type:          req.Type,
				SizeBytes:     req.SizeBytes,
				CreatedBy:     domain.NewUserReference(req.CreatedBy, "", ""),
				CreatedOn:     createdOn,
				StorageKey:    req.StorageKey,
				Status:        req.Status,
			}, nil
		},
	})
	sftpgoClient := &fakeSFTPGoClient{}

	svc := NewCaseService(repo, actorUserRepo(t), nil, sftpgoClient)
	ctx := attachmentCtx(t)

	resp, err := svc.CreateCaseAttachment(ctx, validCreateAttachmentRequest())
	if err != nil {
		t.Fatalf("CreateCaseAttachment returned error: %v", err)
	}
	if capturedReq.CreatedBy != "user-jane" {
		t.Fatalf("expected repo to receive resolved actor id, got %q", capturedReq.CreatedBy)
	}
	if capturedReq.Status != domain.AttachmentStatusComplete {
		t.Fatalf("expected repo to receive status 'complete', got %q", capturedReq.Status)
	}
	if resp.Attachment.ID != testAttachmentID {
		t.Fatalf("expected attachment id %q, got %q", testAttachmentID, resp.Attachment.ID)
	}
	if resp.Attachment.StorageKey == nil || *resp.Attachment.StorageKey == "" {
		t.Fatalf("expected a computed storageKey on the response, got %v", resp.Attachment.StorageKey)
	}
	if resp.Attachment.CreatedBy != "jane.doe@example.com" {
		t.Fatalf("expected createdBy to be the actor's email, got %q", resp.Attachment.CreatedBy)
	}
	if len(sftpgoClient.writtenKeys) != 1 {
		t.Fatalf("expected exactly one SFTPGo write, got %d", len(sftpgoClient.writtenKeys))
	}
	if string(sftpgoClient.writtenBytes[0]) != "hello world" {
		t.Fatalf("expected decoded bytes %q written to SFTPGo, got %q", "hello world", sftpgoClient.writtenBytes[0])
	}
	if sftpgoClient.writtenKeys[0] != *resp.Attachment.StorageKey {
		t.Fatalf("expected SFTPGo write key to match the persisted storageKey: wrote %q, persisted %q", sftpgoClient.writtenKeys[0], *resp.Attachment.StorageKey)
	}
}

// TestCaseService_CreateCaseAttachment_RequiresFile proves this data source
// now requires a base64 file payload, matching the ServiceNow contract --
// there is no more storageKey-supplied-by-caller alternative.
func TestCaseService_CreateCaseAttachment_RequiresFile(t *testing.T) {
	svc := NewCaseService(&stubCaseRepo{}, actorUserRepo(t), nil, &fakeSFTPGoClient{})
	ctx := attachmentCtx(t)

	req := validCreateAttachmentRequest()
	req.File = ""

	_, err := svc.CreateCaseAttachment(ctx, req)
	var ve *apierror.ValidationError
	if !asValidationError(err, &ve) {
		t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
	}
}

// TestCaseService_CreateCaseAttachment_RejectsOversizedFile proves the
// decoded payload is capped at maxAttachmentBytes (10MB), the same limit
// snCaseService.CreateCaseAttachment enforces.
func TestCaseService_CreateCaseAttachment_RejectsOversizedFile(t *testing.T) {
	svc := NewCaseService(&stubCaseRepo{}, actorUserRepo(t), nil, &fakeSFTPGoClient{})
	ctx := attachmentCtx(t)

	oversized := strings.Repeat("a", maxAttachmentBytes+1)
	req := validCreateAttachmentRequest()
	req.File = testDataURI(oversized)

	_, err := svc.CreateCaseAttachment(ctx, req)
	var ve *apierror.ValidationError
	if !asValidationError(err, &ve) {
		t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
	}
}

// TestCaseService_CreateCaseAttachment_RejectsMalformedDataURI proves a File
// value that isn't a "data:...;base64,..." URI is rejected before any SFTPGo
// call is attempted.
func TestCaseService_CreateCaseAttachment_RejectsMalformedDataURI(t *testing.T) {
	sftpgoClient := &fakeSFTPGoClient{}
	svc := NewCaseService(&stubCaseRepo{}, actorUserRepo(t), nil, sftpgoClient)
	ctx := attachmentCtx(t)

	req := validCreateAttachmentRequest()
	req.File = "not-a-data-uri"

	_, err := svc.CreateCaseAttachment(ctx, req)
	var ve *apierror.ValidationError
	if !asValidationError(err, &ve) {
		t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
	}
	if len(sftpgoClient.writtenKeys) != 0 {
		t.Fatalf("expected no SFTPGo write for a malformed data URI, got %d", len(sftpgoClient.writtenKeys))
	}
}

// TestCaseService_CreateCaseAttachment_RejectsNonCaseReferenceType proves
// this data source only models case attachments -- conversation, deployment,
// change_request, and incident have no Postgres schema backing here.
func TestCaseService_CreateCaseAttachment_RejectsNonCaseReferenceType(t *testing.T) {
	svc := NewCaseService(&stubCaseRepo{}, actorUserRepo(t), nil, &fakeSFTPGoClient{})
	ctx := attachmentCtx(t)

	req := validCreateAttachmentRequest()
	req.ReferenceType = domain.ReferenceTypeDeployment

	_, err := svc.CreateCaseAttachment(ctx, req)
	var ve *apierror.ValidationError
	if !asValidationError(err, &ve) {
		t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
	}
}

// TestCaseService_CreateCaseAttachment_RejectsUnauthenticatedCaller proves
// the same "must be a known, authenticated user" gate CreateCaseComment
// already enforces also protects attachment creation.
func TestCaseService_CreateCaseAttachment_RejectsUnauthenticatedCaller(t *testing.T) {
	svc := NewCaseService(&stubCaseRepo{}, stubUserRepo{}, nil, &fakeSFTPGoClient{})
	ctx := contextWithUserIDToken("") // no x-user-id-token header

	_, err := svc.CreateCaseAttachment(ctx, validCreateAttachmentRequest())
	var ue *apierror.UnauthorizedError
	if !errorsAsUnauthorized(err, &ue) {
		t.Fatalf("expected *apierror.UnauthorizedError, got %T: %v", err, err)
	}
}

// TestCaseService_CreateCaseAttachment_RequiresJWTAssertion proves this fails
// closed with an UnauthorizedError when x-jwt-assertion is absent, rather
// than minting a SFTPGo token with an empty credential. This is the expected
// behavior until the BFF is updated to forward x-jwt-assertion through to
// entity-service (see this feature's task file).
func TestCaseService_CreateCaseAttachment_RequiresJWTAssertion(t *testing.T) {
	svc := NewCaseService(caseRepoWithProject(&stubCaseRepo{}), actorUserRepo(t), nil, &fakeSFTPGoClient{})
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com")) // no x-jwt-assertion

	_, err := svc.CreateCaseAttachment(ctx, validCreateAttachmentRequest())
	var ue *apierror.UnauthorizedError
	if !errorsAsUnauthorized(err, &ue) {
		t.Fatalf("expected *apierror.UnauthorizedError, got %T: %v", err, err)
	}
}

// TestCaseService_CreateCaseAttachment_SFTPGoNotConfigured proves a
// deployment without SFTPGO_BASE_URL set fails closed with
// ServiceUnavailableError rather than panicking on a nil client.
func TestCaseService_CreateCaseAttachment_SFTPGoNotConfigured(t *testing.T) {
	svc := NewCaseService(&stubCaseRepo{}, actorUserRepo(t), nil, nil)
	ctx := attachmentCtx(t)

	_, err := svc.CreateCaseAttachment(ctx, validCreateAttachmentRequest())
	var sue *apierror.ServiceUnavailableError
	if !errorsAsServiceUnavailable(err, &sue) {
		t.Fatalf("expected *apierror.ServiceUnavailableError, got %T: %v", err, err)
	}
}

// TestCaseService_CreateCaseAttachment_RollsBackSFTPGoWriteOnRepoFailure
// proves that if the metadata-row insert fails after bytes are already in
// SFTPGo, the orphaned file is cleaned up rather than left behind.
func TestCaseService_CreateCaseAttachment_RollsBackSFTPGoWriteOnRepoFailure(t *testing.T) {
	repo := caseRepoWithProject(&stubCaseRepo{
		createCaseAttachment: func(context.Context, domain.CreateAttachmentRequest) (domain.Attachment, error) {
			return domain.Attachment{}, &apierror.ValidationError{Msg: "one or more referenced IDs do not exist"}
		},
	})
	sftpgoClient := &fakeSFTPGoClient{}
	svc := NewCaseService(repo, actorUserRepo(t), nil, sftpgoClient)
	ctx := attachmentCtx(t)

	_, err := svc.CreateCaseAttachment(ctx, validCreateAttachmentRequest())
	if err == nil {
		t.Fatal("expected an error from the repository failure")
	}
	if len(sftpgoClient.writtenKeys) != 1 {
		t.Fatalf("expected one SFTPGo write attempt, got %d", len(sftpgoClient.writtenKeys))
	}
	if len(sftpgoClient.removedKeys) != 1 || sftpgoClient.removedKeys[0] != sftpgoClient.writtenKeys[0] {
		t.Fatalf("expected rollback to remove the just-written key %q, removed %v", sftpgoClient.writtenKeys[0], sftpgoClient.removedKeys)
	}
}

// TestCaseService_ConfirmCaseAttachment_TransitionsToComplete proves a
// pending row owned by the calling actor is transitioned to complete. This
// path is unchanged by the SFTPGo relay rewrite -- ConfirmCaseAttachment
// itself never touches SFTPGo directly, it only flips the metadata row's
// status.
func TestCaseService_ConfirmCaseAttachment_TransitionsToComplete(t *testing.T) {
	key := testStorageKey
	var confirmedID string
	repo := &stubCaseRepo{
		getCaseAttachmentByID: func(_ context.Context, id string) (domain.Attachment, error) {
			return domain.Attachment{
				ID:         id,
				Status:     domain.AttachmentStatusPending,
				StorageKey: &key,
				CreatedBy:  domain.NewUserReference("user-jane", "jane.doe@example.com", "Jane Doe"),
			}, nil
		},
		confirmCaseAttachment: func(_ context.Context, id string) (domain.Attachment, error) {
			confirmedID = id
			return domain.Attachment{
				ID:         id,
				Status:     domain.AttachmentStatusComplete,
				StorageKey: &key,
				CreatedBy:  domain.NewUserReference("user-jane", "jane.doe@example.com", "Jane Doe"),
			}, nil
		},
	}
	svc := NewCaseService(repo, actorUserRepo(t), nil, nil)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))

	resp, err := svc.ConfirmCaseAttachment(ctx, testAttachmentID)
	if err != nil {
		t.Fatalf("ConfirmCaseAttachment returned error: %v", err)
	}
	if confirmedID != testAttachmentID {
		t.Fatalf("expected repo to receive id %q, got %q", testAttachmentID, confirmedID)
	}
	if resp.Attachment.Status != domain.AttachmentStatusComplete {
		t.Fatalf("expected response status 'complete', got %q", resp.Attachment.Status)
	}
}

// TestCaseService_ConfirmCaseAttachment_RejectsAlreadyComplete proves
// confirming a row that is already 'complete' fails clearly (a ConflictError)
// instead of silently succeeding as a no-op.
func TestCaseService_ConfirmCaseAttachment_RejectsAlreadyComplete(t *testing.T) {
	key := testStorageKey
	repo := &stubCaseRepo{
		getCaseAttachmentByID: func(_ context.Context, id string) (domain.Attachment, error) {
			return domain.Attachment{
				ID:         id,
				Status:     domain.AttachmentStatusComplete,
				StorageKey: &key,
				CreatedBy:  domain.NewUserReference("user-jane", "jane.doe@example.com", "Jane Doe"),
			}, nil
		},
		confirmCaseAttachment: func(context.Context, string) (domain.Attachment, error) {
			t.Fatal("repository mutation should not be reached for an already-complete row")
			return domain.Attachment{}, nil
		},
	}
	svc := NewCaseService(repo, actorUserRepo(t), nil, nil)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))

	_, err := svc.ConfirmCaseAttachment(ctx, testAttachmentID)
	var ce *apierror.ConflictError
	if !errorsAsConflict(err, &ce) {
		t.Fatalf("expected *apierror.ConflictError, got %T: %v", err, err)
	}
}

// TestCaseService_ConfirmCaseAttachment_RejectsDifferentActor proves a user
// other than the one who created the pending row cannot confirm it.
func TestCaseService_ConfirmCaseAttachment_RejectsDifferentActor(t *testing.T) {
	key := testStorageKey
	repo := &stubCaseRepo{
		getCaseAttachmentByID: func(_ context.Context, id string) (domain.Attachment, error) {
			return domain.Attachment{
				ID:         id,
				Status:     domain.AttachmentStatusPending,
				StorageKey: &key,
				CreatedBy:  domain.NewUserReference("someone-else", "someone.else@example.com", "Someone Else"),
			}, nil
		},
		confirmCaseAttachment: func(context.Context, string) (domain.Attachment, error) {
			t.Fatal("repository mutation should not be reached for a non-owning actor")
			return domain.Attachment{}, nil
		},
	}
	svc := NewCaseService(repo, actorUserRepo(t), nil, nil)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))

	_, err := svc.ConfirmCaseAttachment(ctx, testAttachmentID)
	var fe *apierror.ForbiddenError
	if !errorsAsForbidden(err, &fe) {
		t.Fatalf("expected *apierror.ForbiddenError, got %T: %v", err, err)
	}
}

// TestCaseService_ConfirmCaseAttachment_NotFound proves confirming a
// nonexistent attachment surfaces as a NotFoundError.
func TestCaseService_ConfirmCaseAttachment_NotFound(t *testing.T) {
	repo := &stubCaseRepo{
		getCaseAttachmentByID: func(context.Context, string) (domain.Attachment, error) {
			return domain.Attachment{}, &apierror.NotFoundError{Msg: "attachment not found"}
		},
	}
	svc := NewCaseService(repo, actorUserRepo(t), nil, nil)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))

	_, err := svc.ConfirmCaseAttachment(ctx, testAttachmentID)
	var nfe *apierror.NotFoundError
	if !errorsAsNotFound(err, &nfe) {
		t.Fatalf("expected *apierror.NotFoundError, got %T: %v", err, err)
	}
}

// TestCaseService_ConfirmCaseAttachment_RejectsUnauthenticatedCaller proves
// confirming is gated behind the same authentication check as every other
// attachment mutation.
func TestCaseService_ConfirmCaseAttachment_RejectsUnauthenticatedCaller(t *testing.T) {
	repo := &stubCaseRepo{
		getCaseAttachmentByID: func(context.Context, string) (domain.Attachment, error) {
			t.Fatal("repository should not be reached for an unauthenticated caller")
			return domain.Attachment{}, nil
		},
	}
	svc := NewCaseService(repo, stubUserRepo{}, nil, nil)
	ctx := contextWithUserIDToken("")

	_, err := svc.ConfirmCaseAttachment(ctx, testAttachmentID)
	var ue *apierror.UnauthorizedError
	if !errorsAsUnauthorized(err, &ue) {
		t.Fatalf("expected *apierror.UnauthorizedError, got %T: %v", err, err)
	}
}

// TestCaseService_SearchCaseAttachments_ReturnsStorageKey proves the search
// path surfaces storageKey for every row, not just create.
func TestCaseService_SearchCaseAttachments_ReturnsStorageKey(t *testing.T) {
	key := testStorageKey
	repo := &stubCaseRepo{
		searchCaseAttachments: func(_ context.Context, caseID string, pagination domain.Pagination) ([]domain.Attachment, int, error) {
			if caseID != testCaseID {
				t.Fatalf("expected caseID %q, got %q", testCaseID, caseID)
			}
			return []domain.Attachment{{
				ID:            testAttachmentID,
				ReferenceID:   caseID,
				ReferenceType: domain.ReferenceTypeCase,
				Name:          "diagnostics.log",
				Type:          "text/plain",
				SizeBytes:     2048,
				CreatedBy:     domain.NewUserReference("user-jane", "jane.doe@example.com", "Jane Doe"),
				CreatedOn:     time.Now(),
				StorageKey:    &key,
			}}, 1, nil
		},
	}

	svc := NewCaseService(repo, stubUserRepo{}, nil, nil)
	resp, err := svc.SearchCaseAttachments(context.Background(), domain.SearchAttachmentsRequest{
		ReferenceID:   testCaseID,
		ReferenceType: domain.ReferenceTypeCase,
	})
	if err != nil {
		t.Fatalf("SearchCaseAttachments returned error: %v", err)
	}
	if len(resp.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(resp.Attachments))
	}
	if resp.Attachments[0].StorageKey == nil || *resp.Attachments[0].StorageKey != testStorageKey {
		t.Fatalf("expected storageKey %q, got %v", testStorageKey, resp.Attachments[0].StorageKey)
	}
}

// TestCaseService_GetAttachmentByID_ReturnsStorageKeyNotContent proves the
// Postgres-backed GetAttachmentByID never fabricates base64 content: Content
// is always empty and StorageKey is populated, so a caller resolves bytes
// via GetCaseAttachmentContent instead.
func TestCaseService_GetAttachmentByID_ReturnsStorageKeyNotContent(t *testing.T) {
	key := testStorageKey
	repo := &stubCaseRepo{
		getCaseAttachmentByID: func(_ context.Context, id string) (domain.Attachment, error) {
			if id != testAttachmentID {
				t.Fatalf("expected id %q, got %q", testAttachmentID, id)
			}
			return domain.Attachment{
				ID:            id,
				ReferenceID:   testCaseID,
				ReferenceType: domain.ReferenceTypeCase,
				Name:          "diagnostics.log",
				Type:          "text/plain",
				SizeBytes:     2048,
				CreatedBy:     domain.NewUserReference("user-jane", "jane.doe@example.com", "Jane Doe"),
				CreatedOn:     time.Now(),
				StorageKey:    &key,
			}, nil
		},
	}

	svc := NewCaseService(repo, stubUserRepo{}, nil, nil)
	details, err := svc.GetAttachmentByID(context.Background(), testAttachmentID)
	if err != nil {
		t.Fatalf("GetAttachmentByID returned error: %v", err)
	}
	if details.Content != nil {
		t.Fatalf("expected nil Content for a Postgres-sourced attachment, got %q", *details.Content)
	}
	if details.StorageKey == nil || *details.StorageKey != testStorageKey {
		t.Fatalf("expected storageKey %q, got %v", testStorageKey, details.StorageKey)
	}
	if details.CreatedBy != "jane.doe@example.com" {
		t.Fatalf("expected createdBy email, got %q", details.CreatedBy)
	}
	if details.ReferenceID != testCaseID {
		t.Fatalf("expected referenceId %q, got %q", testCaseID, details.ReferenceID)
	}
	if details.ReferenceType == nil || *details.ReferenceType != domain.ReferenceTypeCase {
		t.Fatalf("expected referenceType %q, got %v", domain.ReferenceTypeCase, details.ReferenceType)
	}
}

// TestCaseService_GetAttachmentByID_NotFound proves a missing attachment
// surfaces as a NotFoundError, not a generic error.
func TestCaseService_GetAttachmentByID_NotFound(t *testing.T) {
	repo := &stubCaseRepo{
		getCaseAttachmentByID: func(context.Context, string) (domain.Attachment, error) {
			return domain.Attachment{}, &apierror.NotFoundError{Msg: "attachment not found"}
		},
	}
	svc := NewCaseService(repo, stubUserRepo{}, nil, nil)

	_, err := svc.GetAttachmentByID(context.Background(), testAttachmentID)
	var nfe *apierror.NotFoundError
	if !errorsAsNotFound(err, &nfe) {
		t.Fatalf("expected *apierror.NotFoundError, got %T: %v", err, err)
	}
}

// TestCaseService_GetCaseAttachmentContent_RelaysFromSFTPGo proves this data
// source now relays bytes from SFTPGo server-side, symmetric with
// CreateCaseAttachment and matching snCaseService.GetCaseAttachmentContent's
// shape.
func TestCaseService_GetCaseAttachmentContent_RelaysFromSFTPGo(t *testing.T) {
	key := testStorageKey
	repo := &stubCaseRepo{
		getCaseAttachmentByID: func(_ context.Context, id string) (domain.Attachment, error) {
			return domain.Attachment{ID: id, Type: "text/plain", StorageKey: &key}, nil
		},
	}
	var readKey string
	sftpgoClient := &fakeSFTPGoClient{
		readFile: func(_ context.Context, _, storageKey string) ([]byte, string, error) {
			readKey = storageKey
			return []byte("hello world"), "text/plain", nil
		},
	}
	svc := NewCaseService(repo, actorUserRepo(t), nil, sftpgoClient)
	ctx := attachmentCtx(t)

	content, contentType, err := svc.GetCaseAttachmentContent(ctx, testAttachmentID)
	if err != nil {
		t.Fatalf("GetCaseAttachmentContent returned error: %v", err)
	}
	if string(content) != "hello world" {
		t.Fatalf("expected content %q, got %q", "hello world", content)
	}
	if contentType != "text/plain" {
		t.Fatalf("expected contentType %q, got %q", "text/plain", contentType)
	}
	if readKey != testStorageKey {
		t.Fatalf("expected SFTPGo read at %q, got %q", testStorageKey, readKey)
	}
}

// TestCaseService_GetCaseAttachmentContent_SFTPGoNotConfigured proves a
// deployment without SFTPGO_BASE_URL set fails closed rather than panicking.
func TestCaseService_GetCaseAttachmentContent_SFTPGoNotConfigured(t *testing.T) {
	svc := NewCaseService(&stubCaseRepo{}, stubUserRepo{}, nil, nil)

	content, contentType, err := svc.GetCaseAttachmentContent(context.Background(), testAttachmentID)
	if content != nil {
		t.Fatalf("expected nil content, got %v", content)
	}
	if contentType != "" {
		t.Fatalf("expected empty contentType, got %q", contentType)
	}
	var sue *apierror.ServiceUnavailableError
	if !errorsAsServiceUnavailable(err, &sue) {
		t.Fatalf("expected *apierror.ServiceUnavailableError, got %T: %v", err, err)
	}
}

// TestCaseService_DeleteCaseAttachment_RemovesRow proves delete reaches the
// repository with the requested id and reports success.
func TestCaseService_DeleteCaseAttachment_RemovesRow(t *testing.T) {
	var deletedID string
	repo := &stubCaseRepo{
		deleteCaseAttachment: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}
	svc := NewCaseService(repo, actorUserRepo(t), nil, nil)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))

	resp, err := svc.DeleteCaseAttachment(ctx, domain.DeleteAttachmentRequest{AttachmentID: testAttachmentID})
	if err != nil {
		t.Fatalf("DeleteCaseAttachment returned error: %v", err)
	}
	if deletedID != testAttachmentID {
		t.Fatalf("expected repo to receive id %q, got %q", testAttachmentID, deletedID)
	}
	if resp.Message == "" {
		t.Fatal("expected a non-empty confirmation message")
	}
}

// TestCaseService_DeleteCaseAttachment_RejectsUnauthenticatedCaller proves
// deletion is gated behind the same authentication check as create.
func TestCaseService_DeleteCaseAttachment_RejectsUnauthenticatedCaller(t *testing.T) {
	repo := &stubCaseRepo{
		deleteCaseAttachment: func(context.Context, string) error {
			t.Fatal("repository should not be reached for an unauthenticated caller")
			return nil
		},
	}
	svc := NewCaseService(repo, stubUserRepo{}, nil, nil)
	ctx := contextWithUserIDToken("")

	_, err := svc.DeleteCaseAttachment(ctx, domain.DeleteAttachmentRequest{AttachmentID: testAttachmentID})
	var ue *apierror.UnauthorizedError
	if !errorsAsUnauthorized(err, &ue) {
		t.Fatalf("expected *apierror.UnauthorizedError, got %T: %v", err, err)
	}
}

// TestCaseService_DeleteCaseAttachment_NotFound proves deleting a
// non-existent attachment surfaces as a NotFoundError.
func TestCaseService_DeleteCaseAttachment_NotFound(t *testing.T) {
	repo := &stubCaseRepo{
		deleteCaseAttachment: func(context.Context, string) error {
			return &apierror.NotFoundError{Msg: "attachment not found"}
		},
	}
	svc := NewCaseService(repo, actorUserRepo(t), nil, nil)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))

	_, err := svc.DeleteCaseAttachment(ctx, domain.DeleteAttachmentRequest{AttachmentID: testAttachmentID})
	var nfe *apierror.NotFoundError
	if !errorsAsNotFound(err, &nfe) {
		t.Fatalf("expected *apierror.NotFoundError, got %T: %v", err, err)
	}
}

// TestCaseService_UpdateAttachment_RenamesFile proves the one mutation this
// data source supports (renaming, mirroring the ServiceNow "case" reference
// type behavior) reaches the repository and returns the actor's email.
func TestCaseService_UpdateAttachment_RenamesFile(t *testing.T) {
	updatedOn := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)
	var gotID, gotName, gotUpdatedBy string
	repo := &stubCaseRepo{
		updateAttachmentName: func(_ context.Context, id, name, updatedBy string) (time.Time, error) {
			gotID, gotName, gotUpdatedBy = id, name, updatedBy
			return updatedOn, nil
		},
	}
	svc := NewCaseService(repo, actorUserRepo(t), nil, nil)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))

	name := "renamed.log"
	resp, err := svc.UpdateAttachment(ctx, domain.UpdateAttachmentRequest{
		AttachmentID:  testAttachmentID,
		ReferenceID:   testCaseID,
		ReferenceType: domain.ReferenceTypeCase,
		Name:          &name,
	})
	if err != nil {
		t.Fatalf("UpdateAttachment returned error: %v", err)
	}
	if gotID != testAttachmentID || gotName != "renamed.log" || gotUpdatedBy != "user-jane" {
		t.Fatalf("unexpected repo call: id=%q name=%q updatedBy=%q", gotID, gotName, gotUpdatedBy)
	}
	if resp.Attachment.UpdatedBy != "jane.doe@example.com" {
		t.Fatalf("expected updatedBy email, got %q", resp.Attachment.UpdatedBy)
	}
	if !resp.Attachment.UpdatedOn.Equal(updatedOn) {
		t.Fatalf("expected updatedOn %v, got %v", updatedOn, resp.Attachment.UpdatedOn)
	}
}

// TestCaseService_UpdateAttachment_RejectsDescriptionForCase mirrors the
// ServiceNow path's validateAttachmentUpdate rule: description is not a
// valid field to update for reference type "case".
func TestCaseService_UpdateAttachment_RejectsDescriptionForCase(t *testing.T) {
	svc := NewCaseService(&stubCaseRepo{}, actorUserRepo(t), nil, nil)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))

	name := "renamed.log"
	description := json.RawMessage(`"not allowed"`)
	_, err := svc.UpdateAttachment(ctx, domain.UpdateAttachmentRequest{
		AttachmentID:  testAttachmentID,
		ReferenceID:   testCaseID,
		ReferenceType: domain.ReferenceTypeCase,
		Name:          &name,
		Description:   description,
	})
	var ve *apierror.ValidationError
	if !asValidationError(err, &ve) {
		t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
	}
}

// TestCaseService_UpdateAttachment_RejectsDeploymentReferenceType proves this
// data source rejects the "deployment" reference type ServiceNow allows for
// updates: deployment attachments have no Postgres schema backing here.
func TestCaseService_UpdateAttachment_RejectsDeploymentReferenceType(t *testing.T) {
	svc := NewCaseService(&stubCaseRepo{}, actorUserRepo(t), nil, nil)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))

	name := "renamed.log"
	_, err := svc.UpdateAttachment(ctx, domain.UpdateAttachmentRequest{
		AttachmentID:  testAttachmentID,
		ReferenceID:   testCaseID,
		ReferenceType: domain.ReferenceTypeDeployment,
		Name:          &name,
	})
	var ve *apierror.ValidationError
	if !asValidationError(err, &ve) {
		t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
	}
}

func errorsAsUnauthorized(err error, target **apierror.UnauthorizedError) bool {
	if ue, ok := err.(*apierror.UnauthorizedError); ok {
		*target = ue
		return true
	}
	return false
}

func errorsAsNotFound(err error, target **apierror.NotFoundError) bool {
	if nfe, ok := err.(*apierror.NotFoundError); ok {
		*target = nfe
		return true
	}
	return false
}

func errorsAsServiceUnavailable(err error, target **apierror.ServiceUnavailableError) bool {
	if sue, ok := err.(*apierror.ServiceUnavailableError); ok {
		*target = sue
		return true
	}
	return false
}

func errorsAsConflict(err error, target **apierror.ConflictError) bool {
	if ce, ok := err.(*apierror.ConflictError); ok {
		*target = ce
		return true
	}
	return false
}

func errorsAsForbidden(err error, target **apierror.ForbiddenError) bool {
	if fe, ok := err.(*apierror.ForbiddenError); ok {
		*target = fe
		return true
	}
	return false
}
