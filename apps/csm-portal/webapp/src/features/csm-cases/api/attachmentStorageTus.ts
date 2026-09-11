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

/**
 * Uploads a file directly from the browser to SFTPGo's share-scoped
 * chunked/TUS upload endpoint, bypassing this app's own backend entirely
 * (the backend only mints the write-scoped share — see
 * `usePostCsmCaseAttachment`).
 *
 * Deliberately NOT sent through `useBackendApi`/`useAuthApiClient`: that
 * client only ever attaches a bearer token to this app's own backend (see
 * `useAuthApiClient.ts`), and this upload carries no bearer credential of any
 * kind — the write-scoped share id embedded in the TUS `Upload-Metadata`
 * header is the entire credential (see `UploadFileViaTusInput.shareId`).
 *
 * Follows the TUS resumable-upload protocol (POST to open an upload, then
 * PATCH to send the bytes) against SFTPGo's share-authenticated
 * `POST /api/v2/shares-chunked-uploads` endpoint — confirmed against a live
 * local SFTPGo instance for this change (see this function's inline notes for
 * exactly what was verified).
 */

/** Base64-encodes a UTF-8 string for a TUS `Upload-Metadata` entry. */
function toBase64(value: string): string {
  // btoa operates on a byte string; encode to UTF-8 bytes first so a
  // non-ASCII file name doesn't throw.
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  bytes.forEach((b) => {
    binary += String.fromCharCode(b);
  });
  return btoa(binary);
}

export interface UploadFileViaTusInput {
  sftpgoBaseUrl: string;
  /**
   * The write-scoped SFTPGo share id minted by
   * `POST /cases/{id}/attachments/upload-token`. This is the entire upload
   * credential — no bearer token is ever sent alongside it.
   */
  shareId: string;
  /** The exact SFTPGo path minted by `POST /cases/{id}/attachments/upload-token`. */
  storageKey: string;
  file: File;
  /** Called with 0-100 as the upload progresses. */
  onProgress?: (percent: number) => void;
  signal?: AbortSignal;
}

const TUS_RESUMABLE_VERSION = "1.0.0";

/**
 * Opens a TUS upload session (`POST`) against SFTPGo's share-authenticated
 * `shares-chunked-uploads` endpoint, then uploads the file's bytes in a
 * single `PATCH` at offset 0, reporting progress via `onProgress`. Uses
 * `XMLHttpRequest` for the `PATCH` rather than `fetch`: `fetch` has no
 * cross-browser-reliable upload progress event, while
 * `XMLHttpRequest.upload.onprogress` does.
 */
export async function uploadFileViaTus({
  sftpgoBaseUrl,
  shareId,
  storageKey,
  file,
  onProgress,
  signal,
}: UploadFileViaTusInput): Promise<void> {
  let parsedBaseUrl: URL;
  try {
    parsedBaseUrl = new URL(sftpgoBaseUrl);
  } catch {
    throw new Error(`sftpgoBaseUrl is not a valid URL: "${sftpgoBaseUrl}".`);
  }
  if (parsedBaseUrl.protocol !== "https:") {
    throw new Error(
      `sftpgoBaseUrl must be an https:// URL, got "${sftpgoBaseUrl}".`,
    );
  }
  const trustedOrigin = parsedBaseUrl.origin;

  const createEndpoint = `${sftpgoBaseUrl.replace(/\/+$/, "")}/api/v2/shares-chunked-uploads`;
  // The share's own root already covers storageKey's parent directory (see
  // BeAttachmentUploadTokenResponse.shareId's doc comment), so only the
  // final path segment is sent as "path" here — sending the full storageKey
  // would double up the directory portion and SFTPGo would refuse to write
  // the file.
  const fileName = storageKey.slice(storageKey.lastIndexOf("/") + 1);
  const uploadMetadata = [
    `path ${toBase64(fileName)}`,
    `share_id ${toBase64(shareId)}`,
    `mkdir_parents ${toBase64("true")}`,
  ].join(",");

  // No Authorization header: a write-scoped share id is the entire
  // credential for this endpoint — see UploadFileViaTusInput.shareId's doc
  // comment and the backend's AttachmentStorageHandler.MintUploadToken.
  const createResponse = await fetch(createEndpoint, {
    method: "POST",
    headers: {
      "Tus-Resumable": TUS_RESUMABLE_VERSION,
      "Upload-Length": String(file.size),
      "Upload-Metadata": uploadMetadata,
    },
    signal,
  });
  if (!createResponse.ok) {
    throw new Error(
      `Failed to open the upload session (status ${createResponse.status}).`,
    );
  }

  // The TUS spec returns the upload's URL via `Location`, which may be
  // relative to the create endpoint's origin, or an absolute URL. Either
  // way, the resolved URL's origin must match the configured SFTPGo origin
  // before the PATCH is sent there — otherwise a misconfigured or
  // compromised SFTPGo instance could redirect the upload to an
  // attacker-controlled host.
  const location = createResponse.headers.get("Location");
  const uploadUrl = location
    ? new URL(location, createEndpoint).toString()
    : createEndpoint;
  if (new URL(uploadUrl).origin !== trustedOrigin) {
    throw new Error(
      `Refusing to upload to an untrusted origin returned by the SFTPGo Location header: "${uploadUrl}".`,
    );
  }

  await patchWithProgress({
    url: uploadUrl,
    file,
    onProgress,
    signal,
  });
}

function patchWithProgress({
  url,
  file,
  onProgress,
  signal,
}: {
  url: string;
  file: File;
  onProgress?: (percent: number) => void;
  signal?: AbortSignal;
}): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PATCH", url, true);
    // No Authorization header: the share id embedded in the TUS
    // Upload-Metadata sent to the create call above is the only credential
    // this upload uses — confirmed empirically that SFTPGo's
    // shares-chunked-uploads flow needs none on the follow-up PATCH either.
    xhr.setRequestHeader("Tus-Resumable", TUS_RESUMABLE_VERSION);
    xhr.setRequestHeader("Upload-Offset", "0");
    xhr.setRequestHeader("Content-Type", "application/offset+octet-stream");

    const onAbort = (): void => xhr.abort();
    if (signal) {
      if (signal.aborted) {
        reject(new DOMException("Aborted", "AbortError"));
        return;
      }
      signal.addEventListener("abort", onAbort);
    }

    xhr.upload.onprogress = (event) => {
      if (!onProgress || !event.lengthComputable) return;
      onProgress(Math.round((event.loaded / event.total) * 100));
    };

    xhr.onload = () => {
      signal?.removeEventListener("abort", onAbort);
      if (xhr.status >= 200 && xhr.status < 300) {
        onProgress?.(100);
        resolve();
      } else {
        reject(
          new Error(`Failed to upload the file (status ${xhr.status}).`),
        );
      }
    };
    xhr.onerror = () => {
      signal?.removeEventListener("abort", onAbort);
      reject(new Error("Failed to upload the file (network error)."));
    };
    xhr.onabort = () => {
      signal?.removeEventListener("abort", onAbort);
      reject(new DOMException("Aborted", "AbortError"));
    };

    xhr.send(file);
  });
}
