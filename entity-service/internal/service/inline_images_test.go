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
	"fmt"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

func inlineImgTag(mimeSubtype, payload string) string {
	return fmt.Sprintf(`<img src="data:image/%s;base64,%s">`, mimeSubtype, base64.StdEncoding.EncodeToString([]byte(payload)))
}

// TestCreateCaseComment_NoInlineImages_ContentUnchanged proves a comment with
// no embedded base64 image is persisted unchanged and never reaches SFTPGo.
func TestCreateCaseComment_NoInlineImages_ContentUnchanged(t *testing.T) {
	sftpgoClient := &fakeSFTPGoClient{}
	var persisted domain.CreateCaseCommentRequest
	repo := caseRepoWithProject(&stubCaseRepo{
		createCaseComment: func(_ context.Context, req domain.CreateCaseCommentRequest) (domain.CaseComment, error) {
			persisted = req
			return domain.CaseComment{ID: "comment-1"}, nil
		},
	})
	svc := NewCaseService(repo, actorUserRepo(t), nil, sftpgoClient)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))

	_, err := svc.CreateCaseComment(ctx, domain.CreateCaseCommentRequest{
		CaseID:  testCaseID,
		Type:    domain.CommentTypeComment,
		Content: "plain text, no images",
	})
	if err != nil {
		t.Fatalf("CreateCaseComment returned error: %v", err)
	}
	if persisted.Content != "plain text, no images" {
		t.Fatalf("expected content unchanged, got %q", persisted.Content)
	}
	if len(sftpgoClient.writtenKeys) != 0 {
		t.Fatalf("expected no SFTPGo writes, got %d", len(sftpgoClient.writtenKeys))
	}
}

// TestCreateCaseComment_SingleInlineImage_ExtractedAndRewritten proves one
// embedded base64 image is uploaded to SFTPGo, persisted as a real attachment,
// and the comment's <img> tag is rewritten to a ".iix" reference.
func TestCreateCaseComment_SingleInlineImage_ExtractedAndRewritten(t *testing.T) {
	sftpgoClient := &fakeSFTPGoClient{}
	var persisted domain.CreateCaseCommentRequest
	var createdAttachmentID = "att-generated-1"
	repo := caseRepoWithProject(&stubCaseRepo{
		createCaseComment: func(_ context.Context, req domain.CreateCaseCommentRequest) (domain.CaseComment, error) {
			persisted = req
			return domain.CaseComment{ID: "comment-1"}, nil
		},
		createCaseAttachment: func(_ context.Context, req domain.CreateAttachmentRequest) (domain.Attachment, error) {
			return domain.Attachment{ID: createdAttachmentID, StorageKey: req.StorageKey, Status: req.Status}, nil
		},
	})
	svc := NewCaseService(repo, actorUserRepo(t), nil, sftpgoClient)
	ctx := contextWithUserIDTokenAndJWTAssertion(fakeJWTWithEmail(t, "jane.doe@example.com"), testJWTAssertion)

	content := "before " + inlineImgTag("png", "fake-png-bytes") + " after"
	_, err := svc.CreateCaseComment(ctx, domain.CreateCaseCommentRequest{
		CaseID:  testCaseID,
		Type:    domain.CommentTypeComment,
		Content: content,
	})
	if err != nil {
		t.Fatalf("CreateCaseComment returned error: %v", err)
	}
	if strings.Contains(persisted.Content, "base64,") {
		t.Fatalf("expected persisted content to have base64 image removed, got %q", persisted.Content)
	}
	wantTag := `<img src="/` + createdAttachmentID + `.iix">`
	if !strings.Contains(persisted.Content, wantTag) {
		t.Fatalf("expected persisted content to contain %q, got %q", wantTag, persisted.Content)
	}
	if len(sftpgoClient.writtenKeys) != 1 {
		t.Fatalf("expected exactly one SFTPGo write, got %d", len(sftpgoClient.writtenKeys))
	}
	if string(sftpgoClient.writtenBytes[0]) != "fake-png-bytes" {
		t.Fatalf("expected decoded image bytes %q, got %q", "fake-png-bytes", sftpgoClient.writtenBytes[0])
	}
}

// TestCreateCaseComment_MultipleInlineImages_AllExtracted proves every
// embedded image in a multi-image comment is uploaded and rewritten, not
// just the first.
func TestCreateCaseComment_MultipleInlineImages_AllExtracted(t *testing.T) {
	sftpgoClient := &fakeSFTPGoClient{}
	var attachmentSeq int
	repo := caseRepoWithProject(&stubCaseRepo{
		createCaseComment: func(_ context.Context, req domain.CreateCaseCommentRequest) (domain.CaseComment, error) {
			return domain.CaseComment{ID: "comment-1"}, nil
		},
		createCaseAttachment: func(_ context.Context, req domain.CreateAttachmentRequest) (domain.Attachment, error) {
			attachmentSeq++
			return domain.Attachment{ID: fmt.Sprintf("att-%d", attachmentSeq), StorageKey: req.StorageKey, Status: req.Status}, nil
		},
	})
	svc := NewCaseService(repo, actorUserRepo(t), nil, sftpgoClient)
	ctx := contextWithUserIDTokenAndJWTAssertion(fakeJWTWithEmail(t, "jane.doe@example.com"), testJWTAssertion)

	content := inlineImgTag("png", "image-one") + " middle text " + inlineImgTag("jpeg", "image-two")
	_, err := svc.CreateCaseComment(ctx, domain.CreateCaseCommentRequest{
		CaseID:  testCaseID,
		Type:    domain.CommentTypeComment,
		Content: content,
	})
	if err != nil {
		t.Fatalf("CreateCaseComment returned error: %v", err)
	}
	if len(sftpgoClient.writtenKeys) != 2 {
		t.Fatalf("expected 2 SFTPGo writes, got %d", len(sftpgoClient.writtenKeys))
	}
	if attachmentSeq != 2 {
		t.Fatalf("expected 2 attachment rows created, got %d", attachmentSeq)
	}
}

// TestCreateCaseComment_UnsupportedInlineImageType_Rejected proves an
// unsupported MIME subtype is rejected before any SFTPGo call is made
// (reject-fast, mirroring ServiceNow's own behavior).
func TestCreateCaseComment_UnsupportedInlineImageType_Rejected(t *testing.T) {
	sftpgoClient := &fakeSFTPGoClient{}
	repo := caseRepoWithProject(&stubCaseRepo{})
	svc := NewCaseService(repo, actorUserRepo(t), nil, sftpgoClient)
	ctx := contextWithUserIDTokenAndJWTAssertion(fakeJWTWithEmail(t, "jane.doe@example.com"), testJWTAssertion)

	content := inlineImgTag("gif", "not-allowed")
	_, err := svc.CreateCaseComment(ctx, domain.CreateCaseCommentRequest{
		CaseID:  testCaseID,
		Type:    domain.CommentTypeComment,
		Content: content,
	})
	var ve *apierror.ValidationError
	if !asValidationError(err, &ve) {
		t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
	}
	if len(sftpgoClient.writtenKeys) != 0 {
		t.Fatalf("expected no SFTPGo writes for a rejected type, got %d", len(sftpgoClient.writtenKeys))
	}
}

// TestCreateCaseComment_SecondImageFailure_RollsBackFirst proves that if the
// second of two images fails to upload, the first image's already-created
// attachment row and already-uploaded bytes are rolled back, and the whole
// comment is rejected rather than partially posted.
func TestCreateCaseComment_SecondImageFailure_RollsBackFirst(t *testing.T) {
	var deletedAttachmentIDs []string
	var attachmentSeq int
	sftpgoClient := &fakeSFTPGoClient{
		writeFile: func(_ context.Context, _, storageKey string, _ []byte) error {
			if strings.Contains(storageKey, "image-two") {
				return fmt.Errorf("simulated sftpgo write failure")
			}
			return nil
		},
	}
	repo := caseRepoWithProject(&stubCaseRepo{
		createCaseAttachment: func(_ context.Context, req domain.CreateAttachmentRequest) (domain.Attachment, error) {
			attachmentSeq++
			return domain.Attachment{ID: fmt.Sprintf("att-%d", attachmentSeq), StorageKey: req.StorageKey, Status: req.Status}, nil
		},
		deleteCaseAttachment: func(_ context.Context, id string) error {
			deletedAttachmentIDs = append(deletedAttachmentIDs, id)
			return nil
		},
		createCaseComment: func(context.Context, domain.CreateCaseCommentRequest) (domain.CaseComment, error) {
			t.Fatal("comment should not be persisted when inline-image extraction fails partway through")
			return domain.CaseComment{}, nil
		},
	})
	svc := NewCaseService(repo, actorUserRepo(t), nil, sftpgoClient)
	ctx := contextWithUserIDTokenAndJWTAssertion(fakeJWTWithEmail(t, "jane.doe@example.com"), testJWTAssertion)

	// The second image's filename embeds "image-two" via its generated
	// storage key indirectly -- instead, key off content to control ordering:
	// the fake WriteFile above matches on storageKey substring, but storage
	// keys are randomly generated per-call, so instead simulate failure on
	// the SECOND write call regardless of content.
	callCount := 0
	sftpgoClient.writeFile = func(_ context.Context, _, storageKey string, _ []byte) error {
		callCount++
		if callCount == 2 {
			return fmt.Errorf("simulated sftpgo write failure")
		}
		return nil
	}

	content := inlineImgTag("png", "image-one") + inlineImgTag("jpeg", "image-two")
	_, err := svc.CreateCaseComment(ctx, domain.CreateCaseCommentRequest{
		CaseID:  testCaseID,
		Type:    domain.CommentTypeComment,
		Content: content,
	})
	if err == nil {
		t.Fatal("expected an error from the second image's simulated write failure")
	}
	if len(deletedAttachmentIDs) != 1 || deletedAttachmentIDs[0] != "att-1" {
		t.Fatalf("expected rollback to delete the first image's attachment row (att-1), got %v", deletedAttachmentIDs)
	}
	if len(sftpgoClient.removedKeys) != 1 {
		t.Fatalf("expected rollback to remove the first image's uploaded bytes, got %d removals", len(sftpgoClient.removedKeys))
	}
}
