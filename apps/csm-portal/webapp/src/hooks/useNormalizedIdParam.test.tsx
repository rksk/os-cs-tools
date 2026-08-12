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

import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { JSX } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import "@testing-library/jest-dom/vitest";
import { useNormalizedIdParam } from "@hooks/useNormalizedIdParam";

const DASHED_ID = "56f49f0a-eb1e-c310-fcf5-f5dabad0cdab";
const DASHLESS_ID = "56f49f0aeb1ec310fcf5f5dabad0cdab";

// Renders the real react-router stack (no mock on the navigation hook) so
// these tests prove an actual `replace` navigation happened, not just that
// some navigate function was called with the expected arguments.
function LocationProbe(): JSX.Element {
  const location = useLocation();
  return (
    <div data-testid="location-probe">
      {location.pathname}
      {location.search}
      {location.hash}
    </div>
  );
}

function Probe({ paramName }: { paramName: string }): JSX.Element {
  const id = useNormalizedIdParam(paramName);
  return <div data-testid="hook-result">{id}</div>;
}

function renderAt(
  initialEntry: { pathname: string; search?: string; hash?: string; state?: unknown },
): ReturnType<typeof render> {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <LocationProbe />
      <Routes>
        <Route path="/cases/:caseId" element={<Probe paramName="caseId" />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("useNormalizedIdParam", () => {
  it("returns the dashed id and redirects when the route param is a dashless 32-hex id", async () => {
    renderAt({ pathname: `/cases/${DASHLESS_ID}` });

    expect(screen.getByTestId("hook-result")).toHaveTextContent(DASHED_ID);

    await waitFor(() =>
      expect(screen.getByTestId("location-probe")).toHaveTextContent(
        `/cases/${DASHED_ID}`,
      ),
    );
  });

  it("preserves the query string and hash on the redirect", async () => {
    renderAt({
      pathname: `/cases/${DASHLESS_ID}`,
      search: "?tab=comments",
      hash: "#section-2",
    });

    expect(screen.getByTestId("hook-result")).toHaveTextContent(DASHED_ID);

    await waitFor(() =>
      expect(screen.getByTestId("location-probe")).toHaveTextContent(
        `/cases/${DASHED_ID}?tab=comments#section-2`,
      ),
    );
  });

  it("preserves router state on the redirect", async () => {
    const state = { from: "/cases", parentState: { from: "/dashboard" } };

    render(
      <MemoryRouter
        initialEntries={[{ pathname: `/cases/${DASHLESS_ID}`, state }]}
      >
        <LocationProbe />
        <Routes>
          <Route
            path="/cases/:caseId"
            element={
              <>
                <Probe paramName="caseId" />
                <StateProbe />
              </>
            }
          />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByTestId("hook-result")).toHaveTextContent(DASHED_ID);

    await waitFor(() => {
      expect(screen.getByTestId("location-probe")).toHaveTextContent(
        `/cases/${DASHED_ID}`,
      );
      expect(screen.getByTestId("state-probe")).toHaveTextContent(
        JSON.stringify(state),
      );
    });
  });

  it("returns an already-dashed id unchanged and does not navigate", () => {
    renderAt({ pathname: `/cases/${DASHED_ID}` });

    expect(screen.getByTestId("hook-result")).toHaveTextContent(DASHED_ID);
    expect(screen.getByTestId("location-probe")).toHaveTextContent(
      `/cases/${DASHED_ID}`,
    );
  });

  it.each([
    ["too short (31 hex chars)", DASHLESS_ID.slice(0, 31)],
    ["too long (33 hex chars)", `${DASHLESS_ID}a`],
    ["non-hex characters", `${DASHLESS_ID.slice(0, 30)}gz`],
  ])(
    "returns a malformed id unchanged and does not navigate or crash — %s",
    (_desc, malformedId) => {
      renderAt({ pathname: `/cases/${malformedId}` });

      expect(screen.getByTestId("hook-result")).toHaveTextContent(malformedId);
      expect(screen.getByTestId("location-probe")).toHaveTextContent(
        `/cases/${malformedId}`,
      );
    },
  );
});

function StateProbe(): JSX.Element {
  const location = useLocation();
  return (
    <div data-testid="state-probe">{JSON.stringify(location.state)}</div>
  );
}
