/**
 * Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render as rtlRender, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import type { PagingChainResponse } from "../types";
import { pagingChain, pagingMember } from "../test/fixtures";
import { mayBeResponder, responderNeedsProfilePhoneMessage } from "../utils/pagingPhone";
// vi.mock below is hoisted above this import, so the panel gets the mocks.
import CasePagingPanel from "./CasePagingPanel";

const mutateAsync = vi.fn();
const showError = vi.fn();
const chain: { data?: PagingChainResponse } = {};

function render(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return rtlRender(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

vi.mock("@api/backend/client", () => ({
  BackendApiError: class BackendApiError extends Error {
    status = 0;
  },
  useBackendApi: () => ({}),
}));
vi.mock("@config/apiConfig", () => ({ apiConfig: { backendUrl: "https://example.test" } }));
vi.mock("@context/error-banner/ErrorBannerContext", () => ({ useErrorBanner: () => ({ showError }) }));
const noopMutation = { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false };
vi.mock("../api/useCaseAlertSettings", () => ({
  useCaseAlertSettings: () => ({ data: undefined, isLoading: false, isError: false }),
  canViewCaseAlertSettings: () => false,
  useSaveCaseAlertRoom: () => noopMutation,
  useSetCaseAlertWebhook: () => noopMutation,
  useTestCaseAlertRoom: () => noopMutation,
  useDeleteCaseAlertRoom: () => noopMutation,
  useSaveCaseAlertRoute: () => noopMutation,
  useDeleteCaseAlertRoute: () => noopMutation,
  useSaveCaseAlertValues: () => noopMutation,
  useDeleteCaseAlertValue: () => noopMutation,
}));
vi.mock("../api/usePagingChain", () => ({
  useGetPagingChain: () => ({ data: chain.data, isLoading: false, isError: false, dataUpdatedAt: 0 }),
  usePatchPagingMember: () => ({ mutateAsync }),
  useGetPagingReadiness: () => ({ data: undefined, isLoading: false, isError: false }),
  usePutPagingContact: () => ({ mutate: vi.fn(), isPending: false }),
  useDeletePagingContact: () => ({ mutate: vi.fn(), isPending: false }),
  useTestPagingContact: () => ({ mutate: vi.fn(), isPending: false }),
}));

/** The CRE chain with Ann Poe's profile number as given. */
function chainWithAnn(hasProfilePhone: boolean | undefined): PagingChainResponse {
  const base = pagingChain();
  return {
    ...base,
    members: base.members.map((m) =>
      m.membershipId === "a3" ? pagingMember({ ...m, hasProfilePhone }) : m,
    ),
  };
}

function pickAnnAsSecondResponder() {
  render(<CasePagingPanel readiness={{ days: 7, isLoading: false, isError: false }} />);
  fireEvent.click(screen.getByRole("button", { name: /Edit alert chain/ }));
  fireEvent.change(screen.getByRole("combobox", { name: /2nd responder$/ }), { target: { value: "a3" } });
}

beforeEach(() => {
  mutateAsync.mockReset();
  mutateAsync.mockResolvedValue({});
  showError.mockReset();
});

describe("Level 0 responders need a phone number on their CSM Profile", () => {
  it("refuses someone with no number before asking, and says to add one", () => {
    chain.data = chainWithAnn(false);
    pickAnnAsSecondResponder();
    expect(mutateAsync).not.toHaveBeenCalled();
    expect(showError).toHaveBeenCalledWith(
      "That change to the alert chain was not saved. Ann Poe can't be added to Level 0: a phone number should be included in their CSM Profile (avatar › Profile).",
    );
  });

  it("saves someone with a number", () => {
    chain.data = chainWithAnn(true);
    pickAnnAsSecondResponder();
    expect(showError).not.toHaveBeenCalled();
    expect(mutateAsync).toHaveBeenCalledWith({ membershipId: "a3", responderRank: 2 });
  });

  it("leaves an unknown answer to the backend", () => {
    chain.data = chainWithAnn(undefined);
    pickAnnAsSecondResponder();
    expect(mutateAsync).toHaveBeenCalledWith({ membershipId: "a3", responderRank: 2 });
  });
});

describe("responder phone helpers", () => {
  it("only a known missing number stops a pick", () => {
    expect(mayBeResponder(pagingMember({ membershipId: "x", hasProfilePhone: false }))).toBe(false);
    expect(mayBeResponder(pagingMember({ membershipId: "x", hasProfilePhone: true }))).toBe(true);
    expect(mayBeResponder(pagingMember({ membershipId: "x" }))).toBe(true);
  });

  it("names the person and where the number goes, in the backend's words", () => {
    expect(responderNeedsProfilePhoneMessage("Jane Doe")).toBe(
      "Jane Doe can't be added to Level 0: a phone number should be included in their CSM Profile (avatar › Profile)",
    );
  });
});
