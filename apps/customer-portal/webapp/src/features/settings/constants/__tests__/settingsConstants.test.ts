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

import { describe, expect, it } from "vitest";
import { hasPartnerAccess } from "@features/settings/constants/settingsConstants";

describe("hasPartnerAccess", () => {
  it("accepts legacy prefixed partner roles", () => {
    expect(hasPartnerAccess(["sn_customerservice.partner"])).toBe(true);
    expect(hasPartnerAccess(["sn_customerservice.partner_admin"])).toBe(true);
  });

  it("accepts plain partner roles", () => {
    expect(hasPartnerAccess(["partner"])).toBe(true);
    expect(hasPartnerAccess(["partner_user"])).toBe(true);
  });

  it("accepts plain partner admin", () => {
    expect(hasPartnerAccess(["partner_admin"])).toBe(true);
  });

  it("tolerates case and surrounding whitespace", () => {
    expect(hasPartnerAccess([" Partner_Admin "])).toBe(true);
  });

  it("finds a partner role among other roles", () => {
    expect(hasPartnerAccess(["customer", "partner"])).toBe(true);
  });

  it("rejects non-partner roles", () => {
    expect(hasPartnerAccess(["customer_admin", "admin", "sn_customerservice.customer"])).toBe(false);
    expect(hasPartnerAccess(["partnership"])).toBe(false);
  });

  it("rejects an empty list", () => {
    expect(hasPartnerAccess([])).toBe(false);
  });
});
