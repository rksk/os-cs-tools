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

import { useCallback } from "react";
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from "@tanstack/react-query";
import { ApiQueryKeys } from "@constants/apiConstants";
import { useBackendApi } from "@api/backend/client";
import type {
  BeAttachmentCreatePayload,
  BeAttachmentCreateResponse,
  BeAttachmentSearchPayload,
  BeAttachmentSearchResponse,
  BeDeleteAttachmentResponse,
  BeReferenceType,
} from "@api/backend/types";
import { uiAttachmentFromBe } from "@api/backend/mappers";
import { saveBlob } from "@utils/saveBlob";
import type { CaseAttachment } from "@features/csm-cases/types/csmCases";
import { isSafeAttachmentContentType } from "@features/csm-cases/utils/attachmentPreview";

/**
 * Page size used by the attachments list. A single wide page is enough for the
 * case-detail view. If a case ever exceeds this, switch to an explicit
 * pagination wrapper rather than chasing pages.
 */
const ATTACHMENTS_PAGE_LIMIT = 50;

/**
 * Max upload size in bytes. The BE caps the decoded file at 10 MB (the
 * request body itself is capped higher to allow base64 inflation). Validate
 * here so the user gets a clear message instead of a 413.
 */
export const MAX_ATTACHMENT_SIZE_BYTES = 10 * 1024 * 1024;

/** Read a File as a base64 data URI (`data:<mime>;base64,...`). */
function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") resolve(reader.result);
      else reject(new Error("Failed to read the file."));
    };
    reader.onerror = () =>
      reject(reader.error ?? new Error("Failed to read the file."));
    reader.readAsDataURL(file);
  });
}

/**
 * Load all attachments on a reference entity. Calls `POST /attachments/search`
 * scoped to `referenceType` (defaults to `"case"` for existing call sites) with
 * a single wide page.
 */
export function useGetCsmCaseAttachments(
  caseId: string | undefined,
  referenceType: BeReferenceType = "case",
): UseQueryResult<CaseAttachment[], Error> {
  const api = useBackendApi();

  return useQuery<CaseAttachment[], Error>({
    queryKey: [ApiQueryKeys.CSM_CASE_ATTACHMENTS, referenceType, caseId ?? ""],
    queryFn: async (): Promise<CaseAttachment[]> => {
      if (!caseId) return [];

      const payload: BeAttachmentSearchPayload = {
        referenceId: caseId,
        referenceType,
        pagination: { offset: 0, limit: ATTACHMENTS_PAGE_LIMIT },
      };
      const response = await api.post<
        BeAttachmentSearchPayload,
        BeAttachmentSearchResponse
      >("/attachments/search", payload);
      return response.attachments.map(uiAttachmentFromBe);
    },
    enabled: !!caseId,
    staleTime: 10_000,
  });
}

export interface PostCsmCaseAttachmentInput {
  caseId: string;
  file: File;
  /** Display name for the attachment; defaults to the file's own name. */
  name?: string;
  /** Optional free-text note stored with the attachment. */
  description?: string;
  /** Display name of the uploader (used by the mock; the BE sets its own). */
  uploadedBy: string;
  /** Reference entity type. Defaults to `"case"` for existing call sites. */
  referenceType?: BeReferenceType;
}

export type PostCsmCaseAttachmentResult = UseMutationResult<
  CaseAttachment | null,
  Error,
  PostCsmCaseAttachmentInput
>;

/**
 * Upload a file attachment to a reference entity: the file is sent as a
 * base64 data URI in a single `POST /attachments`. The create response is a
 * thin ack, so the list is refetched on success to hydrate the new entry
 * from search.
 */
export function usePostCsmCaseAttachment(): PostCsmCaseAttachmentResult {
  const api = useBackendApi();
  const queryClient = useQueryClient();

  return useMutation<CaseAttachment | null, Error, PostCsmCaseAttachmentInput>({
    mutationFn: async (input): Promise<CaseAttachment | null> => {
      if (input.file.size > MAX_ATTACHMENT_SIZE_BYTES) {
        throw new Error(
          `"${input.file.name}" is too large. The maximum attachment size is ${
            MAX_ATTACHMENT_SIZE_BYTES / (1024 * 1024)
          } MB.`,
        );
      }

      const referenceType = input.referenceType ?? "case";
      const name = input.name?.trim() || input.file.name;
      const type = input.file.type || "application/octet-stream";

      const dataUri = await readFileAsDataUrl(input.file);
      const payload: BeAttachmentCreatePayload = {
        referenceId: input.caseId,
        referenceType,
        name,
        type,
        file: dataUri,
        description: input.description?.trim() || null,
      };
      await api.post<BeAttachmentCreatePayload, BeAttachmentCreateResponse>(
        "/attachments",
        payload,
      );
      // The create response is a thin ack; refetch hydrates the full entry.
      return null;
    },
    onSuccess: (_created, variables) => {
      void queryClient.invalidateQueries({
        queryKey: [
          ApiQueryKeys.CSM_CASE_ATTACHMENTS,
          variables.referenceType ?? "case",
          variables.caseId,
        ],
      });
    },
  });
}

/**
 * Returns a function that fetches an attachment's raw bytes via
 * `GET /attachments/{id}/content`. The content endpoint always responds with
 * `Content-Disposition: attachment` and streams behind auth, so it is fetched
 * as a `Blob` (a plain `<a href>`/`<img src>` would miss the auth headers and
 * would force a download instead of rendering inline). Shared by the download
 * action and the attachment preview dialog.
 *
 * The BE's response `Content-Type` is only forwarded as-is for an allowlist
 * of known-safe types (`safeAttachmentTypes` in the entity-service's
 * `case_handler.go`) — anything else, including every `video/*` type today,
 * is coerced to `application/octet-stream` to block a stored-XSS via a
 * crafted upstream type. That coercion is correct for the raw endpoint but
 * would make the fetched `Blob` un-renderable by `<img>`/the PDF iframe even
 * though the bytes are fine.
 *
 * The attachment's `contentType` from `/attachments/search` list metadata is
 * whatever the uploader claimed — the BE doesn't coerce it, but it also
 * doesn't verify it against the actual bytes, so it is uploader-controlled
 * and NOT automatically trustworthy. Re-labeling the blob with it would
 * re-enable exactly the coercion the backend allowlist is meant to block
 * (e.g. a file uploaded with a spoofed benign `contentType` but different
 * actual bytes). So the blob is only ever re-labeled when that metadata
 * `contentType` is itself a member of the same backend allowlist, mirrored
 * client-side as {@link isSafeAttachmentContentType} — for anything else the
 * blob is returned as-is (`application/octet-stream`, matching what the
 * backend already coerced it to), which is un-renderable by design.
 */
export function useGetCsmCaseAttachmentContent(): (
  attachment: CaseAttachment,
) => Promise<Blob> {
  const api = useBackendApi();

  return useCallback(
    async (attachment: CaseAttachment): Promise<Blob> => {
      const blob = await api.getBlob(
        `/attachments/${encodeURIComponent(attachment.id)}/content`,
      );
      if (!isSafeAttachmentContentType(attachment.contentType)) return blob;
      return blob.type === attachment.contentType
        ? blob
        : blob.slice(0, blob.size, attachment.contentType);
    },
    [api],
  );
}

/**
 * Returns a function that downloads an attachment, built on
 * {@link useGetCsmCaseAttachmentContent}.
 */
export function useDownloadCsmCaseAttachment(): (
  attachment: CaseAttachment,
) => Promise<void> {
  const getContent = useGetCsmCaseAttachmentContent();

  return useCallback(
    async (attachment: CaseAttachment): Promise<void> => {
      const blob = await getContent(attachment);
      saveBlob(blob, attachment.filename);
    },
    [getContent],
  );
}

export interface DeleteCsmCaseAttachmentInput {
  /** Owning entity id; used only to invalidate the right attachment list. */
  caseId: string;
  attachmentId: string;
  /** Owning entity type. Defaults to `"case"` for existing call sites. */
  referenceType?: BeReferenceType;
}

/**
 * Delete an attachment via `DELETE /attachments/{id}` (ServiceNow data source
 * only). On success the owning entity's attachment list is invalidated so the
 * row drops.
 */
export function useDeleteCsmCaseAttachment(): UseMutationResult<
  void,
  Error,
  DeleteCsmCaseAttachmentInput
> {
  const api = useBackendApi();
  const queryClient = useQueryClient();

  return useMutation<void, Error, DeleteCsmCaseAttachmentInput>({
    mutationFn: async (input): Promise<void> => {
      await api.del<BeDeleteAttachmentResponse>(
        `/attachments/${encodeURIComponent(input.attachmentId)}`,
      );
    },
    onSuccess: (_void, variables) => {
      void queryClient.invalidateQueries({
        queryKey: [
          ApiQueryKeys.CSM_CASE_ATTACHMENTS,
          variables.referenceType ?? "case",
          variables.caseId,
        ],
      });
    },
  });
}
