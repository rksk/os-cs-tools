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
	"encoding/base64"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// maxInlineImageSizeBytes caps a single extracted inline image's decoded
// size. Ported from apps/csm-portal/backend/internal/handler/inline_images.go
// (10MB, matching the webapp's own limit and ServiceNow's
// RichTextUtils.MAX_SIZE_BYTES).
const maxInlineImageSizeBytes = 10 * 1024 * 1024

// allowedInlineImageTypes is the set of data: URI image subtypes extracted
// and stored as real attachments — mirrors ServiceNow's own
// RichTextUtils.processRichTextContent allow-list exactly. Ported unchanged
// from the BFF's inline_images.go.
var allowedInlineImageTypes = map[string]bool{
	"png":  true,
	"jpeg": true,
	"jpg":  true,
	"webp": true,
}

// base64InlineImageTagRe and unsupportedInlineImageTagRe are ported
// unchanged from apps/csm-portal/backend/internal/handler/inline_images.go —
// see that file's own doc comments for the exact matching semantics
// (mirroring ServiceNow's RichTextUtils.base64ImgRegex).
var base64InlineImageTagRe = regexp.MustCompile(`(?i)<img[^>]+src=["']data:(?:@file/|image/)([a-zA-Z0-9.+-]+);base64,([A-Za-z0-9+/=\r\n\s]+)["'][^>]*>`)

var unsupportedInlineImageTagRe = regexp.MustCompile(`(?i)<img[^>]+src=["']data:(?:@file/|image/)([^;]+);base64,[^"']+["'][^>]*>`)

var base64WhitespaceRe = regexp.MustCompile(`\s+`)

// processInlineCommentImages extracts every base64 inline image from
// htmlContent, uploads each to SFTPGo as a real case attachment (storage-keyed
// under the case's project namespace, same convention as
// CreateCaseAttachment/buildStorageKey), and returns HTML with every
// extracted <img> src rewritten to "/<attachmentId>.iix". Returns htmlContent
// unchanged when it carries no base64 inline image.
//
// Ported from apps/csm-portal/backend/internal/handler/inline_images.go's
// InlineImageProcessor.Process, relocated from the BFF into entity-service's
// CreateCaseComment (Postgres) — the extraction/rollback algorithm is
// unchanged; only the write primitive changed (a direct SFTPGo file write
// instead of a share + TUS upload).
//
// Any unsupported inline image MIME subtype anywhere in htmlContent is
// rejected before any image is processed (reject-fast, matching ServiceNow).
// Any failure partway through a multi-image comment rolls back every
// attachment already created earlier in this same call (metadata row +
// SFTPGo bytes), rejecting the whole comment -- a partially-processed
// comment is never posted.
func (s *caseService) processInlineCommentImages(ctx context.Context, caseID, htmlContent string) (string, error) {
	for _, m := range unsupportedInlineImageTagRe.FindAllStringSubmatch(htmlContent, -1) {
		detected := strings.ToLower(m[1])
		if !allowedInlineImageTypes[detected] {
			return "", &apierror.ValidationError{Msg: fmt.Sprintf("unsupported inline image type: %q; allowed types: png, jpeg, jpg, webp", detected)}
		}
	}

	matches := base64InlineImageTagRe.FindAllStringSubmatch(htmlContent, -1)
	if len(matches) == 0 {
		return htmlContent, nil
	}

	if s.sftpgo == nil {
		return "", &apierror.ServiceUnavailableError{Msg: "attachment storage is not configured on this deployment (SFTPGO_BASE_URL unset)"}
	}

	user, err := s.resolveActor(ctx)
	if err != nil {
		return "", err
	}
	accessToken, err := s.mintSFTPGoToken(ctx, user.Email)
	if err != nil {
		return "", err
	}

	// Unrestricted: this is an internal lookup to resolve the case's project
	// for storage-key namespacing, not a caller-facing read -- the caller's
	// authorization to comment on this case is enforced elsewhere.
	caseView, err := s.repo.GetCaseByID(ctx, caseID, repository.SearchScope{Unrestricted: true})
	if err != nil {
		return "", err
	}
	var projectID string
	if caseView.ProjectDetails != nil {
		projectID = caseView.ProjectDetails.ID
	}

	result := htmlContent
	var createdAttachmentIDs []string
	var uploadedStorageKeys []string

	rollback := func() {
		rbCtx := context.WithoutCancel(ctx)
		for _, id := range createdAttachmentIDs {
			if err := s.repo.DeleteCaseAttachment(rbCtx, id); err != nil {
				slog.ErrorContext(rbCtx, "inline-image rollback: DeleteCaseAttachment failed", "attachmentId", id, "err", err)
			}
		}
		for _, key := range uploadedStorageKeys {
			if err := s.sftpgo.RemoveFile(rbCtx, accessToken, key); err != nil {
				slog.ErrorContext(rbCtx, "inline-image rollback: sftpgo RemoveFile failed", "storageKey", key, "err", err)
			}
		}
	}

	for i, m := range matches {
		fullTag := m[0]
		imageType := strings.ToLower(m[1])
		cleanedBase64 := base64WhitespaceRe.ReplaceAllString(m[2], "")

		decoded, err := decodeBase64Flexible(cleanedBase64)
		if err != nil {
			rollback()
			return "", &apierror.ValidationError{Msg: "failed to decode an embedded image"}
		}
		if len(decoded) > maxInlineImageSizeBytes {
			rollback()
			return "", &apierror.ValidationError{Msg: "image exceeds 10MB limit"}
		}

		contentType := "image/" + imageType
		fileName := fmt.Sprintf("inline-image-%d-%d.%s", time.Now().UnixMilli(), i+1, imageType)
		storageKey := buildStorageKey(projectID, caseID, newStorageDirID(), fileName)

		if err := s.sftpgo.WriteFile(ctx, accessToken, storageKey, decoded); err != nil {
			slog.ErrorContext(ctx, "inline-image sftpgo WriteFile failed", "caseId", caseID, "err", err)
			rollback()
			return "", &apierror.ServiceUnavailableError{Msg: "failed to store an embedded image"}
		}
		uploadedStorageKeys = append(uploadedStorageKeys, storageKey)

		a, err := s.repo.CreateCaseAttachment(ctx, domain.CreateAttachmentRequest{
			ReferenceID:   caseID,
			ReferenceType: domain.ReferenceTypeCase,
			Name:          fileName,
			Type:          contentType,
			StorageKey:    &storageKey,
			SizeBytes:     len(decoded),
			Status:        domain.AttachmentStatusComplete,
			CreatedBy:     user.ID,
		})
		if err != nil {
			slog.ErrorContext(ctx, "inline-image CreateCaseAttachment failed", "caseId", caseID, "err", err)
			rollback()
			return "", &apierror.ServiceUnavailableError{Msg: "failed to store an embedded image"}
		}
		createdAttachmentIDs = append(createdAttachmentIDs, a.ID)

		result = strings.Replace(result, fullTag, `<img src="/`+a.ID+`.iix">`, 1)
	}

	return result, nil
}

// decodeBase64Flexible tries standard then URL-safe base64 decoding, mirroring
// snCaseService.CreateCaseAttachment's own fallback for the same reason: some
// clients emit URL-safe base64 for a data: URI payload.
func decodeBase64Flexible(s string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(s)
}
