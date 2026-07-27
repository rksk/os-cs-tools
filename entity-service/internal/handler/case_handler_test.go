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
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

// stubCaseService embeds the service.CaseService interface so tests only need
// to implement the method(s) under test; any call to an unimplemented method
// panics on the nil embedded interface, which is fine since these tests never
// reach them.
type stubCaseService struct {
	service.CaseService
	createAttachmentCalled bool
	createAttachmentResp   domain.CreateAttachmentResponse
	createAttachmentErr    error
}

func (s *stubCaseService) CreateCaseAttachment(_ context.Context, _ domain.CreateAttachmentRequest) (domain.CreateAttachmentResponse, error) {
	s.createAttachmentCalled = true
	return s.createAttachmentResp, s.createAttachmentErr
}

// attachmentRequestBody builds a valid CreateAttachmentRequest JSON body whose
// base64 "file" field is padded to approximately targetBytes total size, so
// tests can exercise the size cap without depending on a real file.
func attachmentRequestBody(t *testing.T, targetBytes int) []byte {
	t.Helper()
	// Reserve room for the JSON envelope; pad only the base64 payload.
	const envelopeOverhead = 512
	payloadBytes := targetBytes - envelopeOverhead
	if payloadBytes < 0 {
		payloadBytes = 0
	}
	raw := make([]byte, payloadBytes)
	encoded := base64.StdEncoding.EncodeToString(raw)

	req := domain.CreateAttachmentRequest{
		ReferenceID:   "11111111-1111-1111-1111-111111111111",
		ReferenceType: domain.ReferenceTypeCase,
		Name:          "test-file.bin",
		Type:          "application/octet-stream",
		File:          encoded,
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return body
}

// TestCreateCaseAttachment_UnderNewLimit_OverOldLimit verifies a body larger
// than the old generic 1 MiB cap, but under the new 15 MiB attachment cap,
// is no longer rejected at the decode stage (this is the QA regression: a
// 2 MB attachment used to fail with "request body too large").
func TestCreateCaseAttachment_UnderNewLimit_OverOldLimit(t *testing.T) {
	body := attachmentRequestBody(t, 2<<20) // ~2 MiB, matches QA's repro size.
	if int64(len(body)) <= maxRequestBodySize {
		t.Fatalf("test body (%d bytes) must exceed the old 1 MiB limit (%d) to be meaningful", len(body), maxRequestBodySize)
	}
	if int64(len(body)) >= maxAttachmentBodySize {
		t.Fatalf("test body (%d bytes) must stay under the new attachment limit (%d)", len(body), maxAttachmentBodySize)
	}

	stub := &stubCaseService{
		createAttachmentResp: domain.CreateAttachmentResponse{Message: "created"},
	}
	h := NewCaseHandler(stub)

	req := httptest.NewRequest(http.MethodPost, "/attachments", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()

	h.CreateCaseAttachment(rec, req)

	if !stub.createAttachmentCalled {
		t.Fatalf("expected CreateCaseAttachment to reach the service layer, got status %d body %q", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCreateCaseAttachment_OverNewLimit verifies a body over the new 15 MiB
// attachment cap is still rejected, with the attachment-specific message.
func TestCreateCaseAttachment_OverNewLimit(t *testing.T) {
	body := attachmentRequestBody(t, 16<<20) // ~16 MiB, over the 15 MiB cap.

	stub := &stubCaseService{}
	h := NewCaseHandler(stub)

	req := httptest.NewRequest(http.MethodPost, "/attachments", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()

	h.CreateCaseAttachment(rec, req)

	if stub.createAttachmentCalled {
		t.Fatalf("expected the request to be rejected before reaching the service layer")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "attachment exceeds the maximum allowed size of 10 MB") {
		t.Fatalf("expected the attachment-specific too-large message, got: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "request body too large") {
		t.Fatalf("expected the generic message to be replaced by the attachment-specific one, got: %s", rec.Body.String())
	}
}
