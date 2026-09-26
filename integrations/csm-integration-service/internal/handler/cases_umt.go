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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/cs-tools/operations/csm-integration-service/internal/apierror"
)

// This file implements POST /updates, the single root-level endpoint added
// for WSO2's internal UMT (product-update-release-management) tool. It
// replaces 3 earlier, separate endpoints (GET /cases/lookup, POST
// /cases/{id}/tags exposed to UMT, POST /cases/{id}/conclude) with one
// caseNumber-keyed call: UMT never needs to resolve or know this platform's
// case UUID at all -- resolution happens inside this handler, as an
// implementation detail, not a separate round-trip the caller must make.
// See CLAUDE.md's "Adding a new endpoint" procedure for the conventions
// followed here.

// caseFieldFilter mirrors entity-service's domain.CaseFieldFilter -- a single
// "field op values" predicate in a case search filter expression.
type caseFieldFilter struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"`
	Values []string `json:"values,omitempty"`
}

// searchCasesByNumberRequest builds the entity-service POST /cases/search body
// for an exact-match lookup by case number, mirroring domain.SearchCasesRequest.
type searchCasesByNumberRequest struct {
	Filters struct {
		Filters []caseFieldFilter `json:"filters"`
	} `json:"filters"`
}

// caseSearchResult is the one field this handler needs out of entity-service's
// case search response -- the platform's own UUID. Deliberately not the full
// case shape: this handler never re-exposes anything else about the matched
// case to the caller.
type caseSearchResult struct {
	ID string `json:"id"`
}

type caseSearchResponse struct {
	Cases []caseSearchResult `json:"cases"`
}

// resolveCaseNumber resolves caseNumber to this platform's own case UUID via
// entity-service's POST /cases/search with a confirmed exact-match field
// filter on "number" -- the same mechanism the earlier, now-removed
// GET /cases/lookup endpoint used, just no longer a separate public call the
// caller has to make. Returns apierror.NotFoundError if no case matches
// (distinct from an upstream transport/search failure, which is returned
// as-is for the caller to see via mapUpstreamError).
func (h *CaseHandler) resolveCaseNumber(w http.ResponseWriter, r *http.Request, number string) (string, bool) {
	var searchReq searchCasesByNumberRequest
	searchReq.Filters.Filters = []caseFieldFilter{
		{Field: "number", Op: "eq", Values: []string{number}},
	}
	body, err := json.Marshal(searchReq)
	if err != nil {
		slog.ErrorContext(r.Context(), "marshal case number filter failed", "err", err)
		writeError(w, http.StatusInternalServerError, ErrMsgInternal)
		return "", false
	}

	raw, err := h.entity.SearchCases(r.Context(), body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchCases (resolve case number) failed", "number", number, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to look up case.")
		return "", false
	}

	var searchResp caseSearchResponse
	if err := json.Unmarshal(raw, &searchResp); err != nil {
		slog.ErrorContext(r.Context(), "unmarshal case search response failed", "number", number, "err", err)
		writeError(w, http.StatusInternalServerError, ErrMsgInternal)
		return "", false
	}
	if len(searchResp.Cases) == 0 {
		writeError(w, http.StatusNotFound, ErrMsgNotFound)
		return "", false
	}

	return searchResp.Cases[0].ID, true
}

// updatesRequest is the caller-facing request body for POST /updates. Every
// field but CaseNumber is optional; at least one action field must be
// present. MarkFixIssued is a *bool so absent (nil) is distinguishable from
// an explicit false, which is rejected -- entity-service's markFixIssued
// field only ever accepts true, matching Acknowledge's own semantics
// elsewhere in this platform.
type updatesRequest struct {
	CaseNumber       string `json:"caseNumber"`
	Label            string `json:"label,omitempty"`
	BestCaseFixEta   string `json:"bestCaseFixEta,omitempty"`
	MostLikelyFixEta string `json:"mostLikelyFixEta,omitempty"`
	WorstCaseFixEta  string `json:"worstCaseFixEta,omitempty"`
	Comment          string `json:"comment,omitempty"`
	MarkFixIssued    *bool  `json:"markFixIssued,omitempty"`
	UpdateLevel      string `json:"updateLevel,omitempty"`
}

// legOutcome reports one leg of a composite operation's outcome independently.
// Result carries the raw upstream response on success (omitted on failure);
// Error carries a short, log-safe summary on failure (omitted on success) --
// never the raw upstream error body, per this service's error-message
// convention (see response.go).
type legOutcome struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// updatesResponse is the response body for POST /updates. Only the legs the
// caller actually requested are present -- a *legOutcome left nil (via
// omitempty) rather than every possible leg always appearing, so the
// response shape reflects exactly what was asked for.
type updatesResponse struct {
	CaseID        string      `json:"caseId"`
	Label         *legOutcome `json:"label,omitempty"`
	FixEta        *legOutcome `json:"fixEta,omitempty"`
	MarkFixIssued *legOutcome `json:"markFixIssued,omitempty"`
	Comment       *legOutcome `json:"comment,omitempty"`
}

// addCaseTagUpstreamRequest is the entity-service POST /cases/{id}/tags body
// this handler builds, mirroring domain.AddCaseTagRequest. ActorEmail is
// always this service's own configured M2M identity -- never taken from the
// caller, since that would let any caller claim to be any user.
type addCaseTagUpstreamRequest struct {
	Label      string `json:"label"`
	ActorEmail string `json:"actorEmail"`
}

// fixEtaPatchBody is the entity-service PATCH /cases/{id} body sent by the
// fix-ETA leg. Only the fields the caller actually supplied are set --
// entity-service treats each of the 3 fix-ETA fields as independently
// optional within this combinable group.
type fixEtaPatchBody struct {
	BestCaseFixEta   string `json:"bestCaseFixEta,omitempty"`
	MostLikelyFixEta string `json:"mostLikelyFixEta,omitempty"`
	WorstCaseFixEta  string `json:"worstCaseFixEta,omitempty"`
}

// markFixIssuedPatchBody is the entity-service PATCH /cases/{id} body sent by
// the markFixIssued leg. Sent as its own, separate PATCH call from the
// fix-ETA leg above -- entity-service's UpdateCase dispatch keeps
// markFixIssued mutually exclusive from every other field on the same
// request, so the two can never be combined into one upstream call.
type markFixIssuedPatchBody struct {
	MarkFixIssued bool `json:"markFixIssued"`
}

// updatesCommentBody is the entity-service POST /cases/{id}/comments body
// sent by the comment leg, mirroring domain.CreateCaseCommentRequest.
// ActorEmail is always this service's own configured M2M identity -- this
// handler builds and sends this body directly to the entity client, it does
// not go through CaseHandler.CreateCaseComment's own actorEmail-injection
// logic, so this leg injects it itself.
type updatesCommentBody struct {
	Type       string `json:"type"`
	Content    string `json:"content"`
	ActorEmail string `json:"actorEmail"`
}

// Updates handles POST /updates, the single entry point WSO2's internal UMT
// tool uses for everything it needs to do to a case once it has a case
// number in hand: add a label, set fix-ETA estimates, mark the fix as
// issued, and/or add a closing comment -- any combination, in one call.
//
// Processing order:
//  1. Resolve CaseNumber to this platform's case UUID (a real prerequisite,
//     not a "leg" -- a 404 here fails the whole request, since nothing else
//     can proceed without a resolved case).
//  2. Attempt every requested action leg independently, exactly like the
//     earlier ConcludeCase endpoint this replaces: each leg's outcome is
//     reported on its own (success/result, or success:false/error), and one
//     leg's failure never hides or blocks another's success. Label and
//     fix-ETA and markFixIssued are 3 separate upstream calls even when all
//     requested together -- entity-service's UpdateCase dispatch keeps
//     markFixIssued mutually exclusive from the fix-ETA combinable group, so
//     they can never share one PATCH call.
//
// Returns 200 whenever the request itself is well-formed (a well-formed
// request whose every leg fails is still a 200 reporting those failures --
// the leg outcomes are the payload, not an HTTP-level error); a malformed
// request (missing caseNumber, no action field at all, invalid markFixIssued
// value, oversized/invalid body) gets an ordinary 4xx, and an unresolvable
// caseNumber gets a 404.
func (h *CaseHandler) Updates(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	if !json.Valid(raw) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	var req updatesRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	number := strings.TrimSpace(req.CaseNumber)
	if number == "" {
		writeError(w, http.StatusBadRequest, ErrMsgCaseNumberRequired)
		return
	}

	hasLabel := strings.TrimSpace(req.Label) != ""
	hasFixEta := req.BestCaseFixEta != "" || req.MostLikelyFixEta != "" || req.WorstCaseFixEta != ""
	hasComment := strings.TrimSpace(req.Comment) != ""
	hasMarkFixIssued := req.MarkFixIssued != nil
	if !hasLabel && !hasFixEta && !hasComment && !hasMarkFixIssued {
		writeError(w, http.StatusBadRequest, ErrMsgUpdatesActionRequired)
		return
	}
	if hasMarkFixIssued && !*req.MarkFixIssued {
		writeError(w, http.StatusBadRequest, ErrMsgMarkFixIssuedMustBeTrue)
		return
	}

	caseID, ok := h.resolveCaseNumber(w, r, number)
	if !ok {
		return
	}

	resp := updatesResponse{CaseID: caseID}
	if hasLabel {
		outcome := h.updatesAddLabel(r, caseID, req.Label)
		resp.Label = &outcome
	}
	if hasFixEta {
		outcome := h.updatesSetFixEta(r, caseID, req)
		resp.FixEta = &outcome
	}
	if hasMarkFixIssued {
		outcome := h.updatesMarkFixIssued(r, caseID)
		resp.MarkFixIssued = &outcome
	}
	if hasComment {
		outcome := h.updatesAddComment(r, caseID, req)
		resp.Comment = &outcome
	}

	respBody, err := json.Marshal(resp)
	if err != nil {
		slog.ErrorContext(r.Context(), "marshal updates response failed", "caseId", caseID, "err", err)
		writeError(w, http.StatusInternalServerError, ErrMsgInternal)
		return
	}
	writeJSON(w, http.StatusOK, respBody)
}

// updatesAddLabel runs the label leg.
func (h *CaseHandler) updatesAddLabel(r *http.Request, caseID, label string) legOutcome {
	body, err := json.Marshal(addCaseTagUpstreamRequest{Label: label, ActorEmail: h.umtActorEmail})
	if err != nil {
		slog.ErrorContext(r.Context(), "marshal add case tag body failed", "caseID", caseID, "err", err)
		return legOutcome{Success: false, Error: ErrMsgInternal}
	}

	result, err := h.entity.AddCaseTag(r.Context(), caseID, body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity AddCaseTag failed", "caseID", caseID, "err", summarizeErr(err))
		return legOutcome{Success: false, Error: legErrorMessage(err)}
	}
	return legOutcome{Success: true, Result: json.RawMessage(result)}
}

// updatesSetFixEta runs the fix-ETA leg -- only the fields the caller
// actually supplied are sent upstream.
func (h *CaseHandler) updatesSetFixEta(r *http.Request, caseID string, req updatesRequest) legOutcome {
	body, err := json.Marshal(fixEtaPatchBody{
		BestCaseFixEta:   req.BestCaseFixEta,
		MostLikelyFixEta: req.MostLikelyFixEta,
		WorstCaseFixEta:  req.WorstCaseFixEta,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "marshal fix-ETA patch body failed", "caseID", caseID, "err", err)
		return legOutcome{Success: false, Error: ErrMsgInternal}
	}

	result, err := h.entity.PatchCase(r.Context(), caseID, body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity PatchCase (fix-ETA) failed", "caseID", caseID, "err", summarizeErr(err))
		return legOutcome{Success: false, Error: legErrorMessage(err)}
	}
	return legOutcome{Success: true, Result: json.RawMessage(result)}
}

// updatesMarkFixIssued runs the markFixIssued leg. Always its own PATCH call,
// separate from the fix-ETA leg -- see this file's doc comment for why the
// two can never be combined into one upstream request.
func (h *CaseHandler) updatesMarkFixIssued(r *http.Request, caseID string) legOutcome {
	body, err := json.Marshal(markFixIssuedPatchBody{MarkFixIssued: true})
	if err != nil {
		slog.ErrorContext(r.Context(), "marshal markFixIssued patch body failed", "caseID", caseID, "err", err)
		return legOutcome{Success: false, Error: ErrMsgInternal}
	}

	result, err := h.entity.PatchCase(r.Context(), caseID, body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity PatchCase (markFixIssued) failed", "caseID", caseID, "err", summarizeErr(err))
		return legOutcome{Success: false, Error: legErrorMessage(err)}
	}
	return legOutcome{Success: true, Result: json.RawMessage(result)}
}

// updatesAddComment runs the comment leg. UpdateLevel, when present, is
// folded into the comment content -- entity-service's comment body has no
// separate field for it.
func (h *CaseHandler) updatesAddComment(r *http.Request, caseID string, req updatesRequest) legOutcome {
	content := req.Comment
	if req.UpdateLevel != "" {
		content = fmt.Sprintf("[Update Level: %s] %s", req.UpdateLevel, req.Comment)
	}

	body, err := json.Marshal(updatesCommentBody{Type: "comment", Content: content, ActorEmail: h.umtActorEmail})
	if err != nil {
		slog.ErrorContext(r.Context(), "marshal updates comment body failed", "caseID", caseID, "err", err)
		return legOutcome{Success: false, Error: ErrMsgInternal}
	}

	result, err := h.entity.CreateCaseComment(r.Context(), caseID, body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity CreateCaseComment (updates) failed", "caseID", caseID, "err", summarizeErr(err))
		return legOutcome{Success: false, Error: legErrorMessage(err)}
	}
	return legOutcome{Success: true, Result: json.RawMessage(result)}
}

// legErrorMessage maps an upstream error to a short, caller-safe summary for
// a composite leg outcome, following the same status-code mapping as
// mapUpstreamError but returning a string instead of writing an HTTP
// response -- never the raw upstream error body.
func legErrorMessage(err error) string {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized:
			return ErrMsgUnauthorized
		case http.StatusForbidden:
			return ErrMsgForbidden
		case http.StatusNotFound:
			return ErrMsgNotFound
		case http.StatusBadRequest:
			return ErrMsgBadRequest
		case http.StatusConflict, http.StatusUnprocessableEntity:
			return fmt.Sprintf("Upstream returned status %d.", apiErr.StatusCode)
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return "Upstream service unavailable."
		default:
			return ErrMsgInternal
		}
	}
	return ErrMsgInternal
}
