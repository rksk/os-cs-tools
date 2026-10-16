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

import { describe, expect, it } from "vitest";
import { projectContactRoleLabels } from "@features/csm-projects/utils/projectContactRoles";

describe("projectContactRoleLabels", () => {
  it("does not show Admin for an account admin whose project groups are only Full Access", () => {
    expect(
      projectContactRoleLabels({
        roles: ["PORTAL_USER", "SECURITY_CONTACT"],
        accountRoles: ["customer", "customer_admin", "external"],
      }),
    ).toEqual(["Portal User", "Security User"]);
  });

  it("shows Admin from the project ADMIN role, once", () => {
    expect(
      projectContactRoleLabels({
        roles: ["ADMIN", "PORTAL_USER"],
        accountRoles: ["customer_admin"],
      }),
    ).toEqual(["Admin", "Portal User"]);
  });

  it("adds the portal fallback for a project admin with no portal role", () => {
    expect(
      projectContactRoleLabels({
        roles: ["ADMIN"],
      }),
    ).toEqual(["Admin", "Portal User"]);
  });

  it("keeps BUSINESS_CONTACT beside the portal fallback", () => {
    expect(
      projectContactRoleLabels({
        roles: ["BUSINESS_CONTACT"],
        accountRoles: ["partner", "partner_admin", "external"],
      }),
    ).toEqual(["Portal User", "Business Contact"]);
  });

  it("shows a system user without Portal User or Security User", () => {
    expect(
      projectContactRoleLabels({
        roles: ["PORTAL_USER", "SECURITY_CONTACT", "LEAD_USER"],
        isCsIntegrationUser: true,
      }),
    ).toEqual(["Lead", "System User"]);
  });

  it("does not invent Portal User for a confirmed unlinked row with no roles", () => {
    expect(
      projectContactRoleLabels({
        roles: [],
        customerContactPresent: false,
      }),
    ).toEqual([]);
  });

  it("falls back to Portal User when the role is not one the customer portal shows", () => {
    expect(projectContactRoleLabels({ roles: ["viewer"] })).toEqual(["Portal User"]);
  });
});
