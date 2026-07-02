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
import { useAsgardeo } from "@asgardeo/react";
import { apiConfig } from "@config/apiConfig";
import { AUTH_NOT_READY_ERROR_MESSAGE } from "@constants/apiConstants";
import { useLogger } from "@hooks/useLogger";
import { CORRELATION_ID_HEADER, newCorrelationId } from "@utils/correlationId";

// The SDK does not re-export its HTTP types, and importing `@asgardeo/browser`
// directly would add an undeclared (pnpm-non-hoisted) dependency. Infer the
// request-config and response shapes from the bound `http.request` signature
// instead, so they track the installed SDK version with no extra import.
type AsgardeoHttp = ReturnType<typeof useAsgardeo>["http"];
type HttpRequestConfig = NonNullable<Parameters<AsgardeoHttp["request"]>[0]>;

// Axios-style rejection shape thrown by the SDK on non-2xx responses.
interface SdkHttpError {
  code?: string;
  response?: {
    data?: unknown;
    headers?: Record<string, string>;
    status: number;
    statusText?: string;
  };
}

/**
 * True when a token call failed because the auth SDK had not finished
 * initializing yet (code `SPA-AUTH_CLIENT-VM-NF01`, "The SDK must be
 * initialized first"). This is a transient race on first paint — the silent
 * refresh can ask for a token a tick before the SDK is ready — so callers
 * should treat it as "auth not ready, retry", not a hard error.
 */
function isSdkNotInitializedError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  if ((error as { code?: string }).code === "SPA-AUTH_CLIENT-VM-NF01") {
    return true;
  }
  return /SDK (?:must be initialized|is not initialized)/i.test(
    `${error.name} ${error.message}`,
  );
}

// Origin we are willing to attach credentials to. Computed once at module load
// so we don't accidentally send the ID token anywhere else. The bearer itself
// is injected inside the auth worker (never in this thread), but the worker's
// HTTP handler has no resource-server allowlist of its own, so we keep this
// guard to refuse any non-backend URL before it reaches the worker.
const trustedBackendOrigin = (() => {
  try {
    return new URL(apiConfig.backendUrl).origin;
  } catch {
    return "";
  }
})();

function resolveRequestUrl(input: RequestInfo | URL): URL {
  if (input instanceof Request) return new URL(input.url, window.location.origin);
  if (input instanceof URL) return input;
  return new URL(input.toString(), window.location.origin);
}

// Statuses that MUST carry a null body — the Response constructor throws if a
// body is supplied alongside them.
const NULL_BODY_STATUSES = new Set([101, 103, 204, 205, 304]);

/**
 * Merge the caller's headers (from an outer `Request` and/or `RequestInit`)
 * into a plain record, then stamp the identity + tracing headers the backend
 * expects. Returns a `Record<string, string>` because the SDK's HTTP config
 * takes plain-object headers, not a `Headers` instance.
 *
 * `Authorization` is deliberately NOT set here: with web-worker token storage
 * the access token lives in the worker and is attached there (`attachToken`),
 * so it never enters this thread.
 */
function buildRequestHeaders(
  input: RequestInfo | URL,
  options: RequestInit | undefined,
  idToken: string,
  correlationId: string,
): Record<string, string> {
  const headers: Record<string, string> = {};
  const seed = (source: HeadersInit | undefined) => {
    if (!source) return;
    new Headers(source).forEach((value, key) => {
      headers[key] = value;
    });
  };
  // When `input` is a Request, seed from it first and let option-level headers
  // override (matching fetch's precedence).
  if (input instanceof Request) seed(input.headers);
  seed(options?.headers);

  // The ID token travels alongside the (worker-attached) access token — same
  // convention as the customer portal: the gateway validates the bearer, while
  // the backend reads the user's identity claims from `x-user-id-token`.
  headers["x-user-id-token"] = idToken;
  // Correlation ID for end-to-end tracing. The backend honours an inbound value
  // and only generates its own when absent, so a caller-supplied header (rare:
  // a retry that wants to reuse an ID) is preserved; otherwise we stamp a fresh
  // per-request UUID.
  if (!Object.keys(headers).some((k) => k.toLowerCase() === CORRELATION_ID_HEADER.toLowerCase())) {
    headers[CORRELATION_ID_HEADER] = correlationId;
  }
  if (!Object.keys(headers).some((k) => k.toLowerCase() === "accept")) {
    headers["Accept"] = "application/json";
  }
  return headers;
}

/**
 * Resolve the request body into the SDK's `data` field. Callers hand us a
 * pre-serialized JSON string (`JSON.stringify(payload)`); the SDK re-serializes
 * `data` with `JSON.stringify`, so we must parse the string back to an object
 * to avoid double-encoding. Non-JSON string bodies (none today) pass through
 * untouched.
 */
function resolveRequestData(body: BodyInit | null | undefined): unknown {
  if (body == null || body === "") return undefined;
  if (typeof body !== "string") return body; // FormData/Blob/etc. — pass through
  try {
    return JSON.parse(body);
  } catch {
    return body;
  }
}

/**
 * Rebuild a real `fetch` `Response` from the SDK's axios-shaped
 * {@link HttpResponse} (or the `response` on a thrown {@link HttpError}), so
 * every existing caller keeps using the standard `Response` API
 * (`.ok` / `.status` / `.json()` / `.blob()` / `.headers.get()`) unchanged.
 *
 * The SDK parses the body by content-type: JSON responses arrive as an object,
 * everything else as a string. We re-stringify objects so `.json()` round-trips
 * and hand strings through verbatim so `.text()` / `.blob()` see the raw body.
 */
function toFetchResponse(result: {
  data?: unknown;
  status: number;
  statusText?: string;
  headers?: Record<string, string>;
}): Response {
  const headers = new Headers(result.headers ?? {});
  const status = result.status;
  const body =
    NULL_BODY_STATUSES.has(status) || result.data == null
      ? null
      : typeof result.data === "string"
        ? result.data
        : JSON.stringify(result.data);
  return new Response(body, {
    status,
    statusText: result.statusText ?? "",
    headers,
  });
}

// Auth-aware request helper that mirrors the previous `fetch`-based signature:
// it returns a real `Response` so callers are unchanged. Under the hood it goes
// through the auth SDK's HTTP handler (`http.request`), which — with web-worker
// token storage — injects the access token INSIDE the worker as
// `Authorization: Bearer …`. The Choreo gateway validates that token and
// forwards it upstream as `x-jwt-assertion`, which csm-portal-backend reads in
// its auth middleware; `x-user-id-token` passes through to the backend
// untouched. Requests to any origin other than the configured backend are
// refused so credentials can't be leaked to third-party hosts.
export function useAuthApiClient() {
  const { http, getIdToken } = useAsgardeo();
  const logger = useLogger();

  return useCallback(
    async (input: RequestInfo | URL, options?: RequestInit): Promise<Response> => {
      const url = resolveRequestUrl(input);
      if (!trustedBackendOrigin || url.origin !== trustedBackendOrigin) {
        throw new Error(
          `Refusing to send access token to untrusted origin ${url.origin}`,
        );
      }

      let idToken: string | undefined;
      try {
        idToken = await getIdToken();
      } catch (error) {
        // Normalise the SDK-not-initialized race into the shared "auth not
        // ready" signal so callers warn-and-retry instead of surfacing a raw
        // AsgardeoAuthException as a hard error.
        if (isSdkNotInitializedError(error)) {
          throw new Error(AUTH_NOT_READY_ERROR_MESSAGE);
        }
        throw error;
      }
      if (!idToken) {
        throw new Error("Unable to retrieve ID token");
      }

      // One correlation ID per physical request (React Query retries each get a
      // distinct one, matching the backend's per-request unit). A caller that
      // pre-set the header keeps its value; we log whichever ID actually ships.
      const headers = buildRequestHeaders(
        input,
        options,
        idToken,
        newCorrelationId(),
      );
      const method = (
        options?.method ??
        (input instanceof Request ? input.method : "GET")
      ).toUpperCase();
      const rawBody =
        options?.body ?? (input instanceof Request ? input.body : undefined);
      const data = resolveRequestData(rawBody);

      const requestConfig: HttpRequestConfig = {
        url: url.toString(),
        method,
        headers,
        // Attach the bearer inside the worker; never expose it to this thread.
        attachToken: true,
        // The SDK's HTTP client defaults to `credentials: "include"`, which
        // forces a credentialed CORS request and demands
        // `Access-Control-Allow-Credentials: true` from the gateway (which it
        // does not send). We authenticate via the bearer header only, never
        // cookies, so omit credentials — matching the previous fetch behaviour
        // and keeping the preflight simple.
        credentials: "omit",
        signal: options?.signal ?? undefined,
      };
      if (data !== undefined) requestConfig.data = data;

      const correlationId = headers[CORRELATION_ID_HEADER] ?? "";
      const logLine = (status: number | string) =>
        `[api] ${method} ${url.pathname} -> ${status} correlationID=${correlationId}`;

      try {
        const result = await http.request(requestConfig);
        const response = toFetchResponse(result);
        logger.debug(logLine(response.status));
        return response;
      } catch (error) {
        // The SDK rejects on non-2xx (axios-style). Rebuild a `Response` from
        // the error's payload so callers' existing `if (!res.ok)` handling runs
        // unchanged; only a genuine transport failure (no `response`) rethrows.
        if (isSdkNotInitializedError(error)) {
          throw new Error(AUTH_NOT_READY_ERROR_MESSAGE);
        }
        const httpError = error as SdkHttpError;
        if (httpError?.response && typeof httpError.response.status === "number") {
          const response = toFetchResponse(httpError.response);
          logger.error(logLine(response.status));
          return response;
        }
        logger.error(logLine("network error"), error);
        throw error;
      }
    },
    [http, getIdToken, logger],
  );
}
