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
import {
  getAffectedProductGroupKey,
  getAffectedProducts,
  groupAffectedProducts,
  getVulnerabilitySeverityColor,
  getVulnerabilityStatusColor,
} from "@features/security/utils/vulnerabilities";
import type { ProductVulnerability } from "@features/security/types/security";

describe("vulnerabilities utils", () => {
  it("maps severity labels to theme colors", () => {
    expect(getVulnerabilitySeverityColor(" Critical ")).toBe("error.main");
    expect(getVulnerabilitySeverityColor("HIGH")).toBe("warning.main");
    expect(getVulnerabilitySeverityColor("unknown")).toBe("text.secondary");
  });

  it("maps status labels to theme colors with strict matching", () => {
    expect(getVulnerabilityStatusColor("in progress")).toBe("warning.main");
    expect(getVulnerabilityStatusColor("resolved")).toBe("success.main");
    expect(getVulnerabilityStatusColor("unresolved")).toBe("text.secondary");
  });

  describe("getAffectedProducts", () => {
    const make = (
      overrides: Partial<ProductVulnerability>,
    ): ProductVulnerability => ({
      id: "id",
      cveId: "CVE-2026-1",
      vulnerabilityId: "VULN-1",
      severity: { id: "1", label: "High" },
      productName: "Product A",
      productVersion: "1.0.0",
      componentName: "lib-a",
      version: "1.0",
      type: "library",
      useCase: null,
      justification: null,
      resolution: null,
      ...overrides,
    });

    it("returns every record that shares the CVE, ordered by product and version", () => {
      const current = make({ id: "a2", productName: "Product A", productVersion: "10.0.0" });
      const all = [
        current,
        make({ id: "b1", productName: "Product B", productVersion: "2.0.0" }),
        make({ id: "a1", productName: "Product A", productVersion: "2.0.0" }),
        make({ id: "other", cveId: "CVE-2026-2", productName: "Product A" }),
      ];

      expect(getAffectedProducts(all, current).map((v) => v.id)).toEqual([
        "a1",
        "a2",
        "b1",
      ]);
    });

    it("matches the CVE case-insensitively and ignores surrounding whitespace", () => {
      const current = make({ id: "a", cveId: "CVE-2026-1" });
      const all = [current, make({ id: "b", cveId: " cve-2026-1 " })];

      expect(getAffectedProducts(all, current)).toHaveLength(2);
    });

    it("always includes the current record, even when the list does not contain it", () => {
      const current = make({ id: "missing" });
      const all = [make({ id: "x" })];

      // Same product/version/component, so the id is the final tie-breaker.
      expect(getAffectedProducts(all, current).map((v) => v.id)).toEqual([
        "missing",
        "x",
      ]);
    });

    it("lists each id once and prefers the freshly fetched current record over its list copy", () => {
      const stale = make({ id: "a", productVersion: "1.0.0" });
      const fresh = make({ id: "a", productVersion: "9.9.9" });
      const all = [stale, make({ id: "b", productVersion: "2.0.0" }), make({ id: "b", productVersion: "2.0.0" })];

      const result = getAffectedProducts(all, fresh);

      // Ordered by product version: 2.0.0 (b) comes before the fresh 9.9.9 (a).
      expect(result.map((v) => v.id)).toEqual(["b", "a"]);
      expect(result.find((v) => v.id === "a")?.productVersion).toBe("9.9.9");
      expect(result.filter((v) => v.id === "b")).toHaveLength(1);
    });

    it("lists the same group whichever record is opened, even when only some records have a CVE", () => {
      const noCve = make({ id: "r1", cveId: "", vulnerabilityId: "V9", productName: "A" });
      const withCve = make({ id: "r2", cveId: "CVE-1", vulnerabilityId: "V9", productName: "B" });
      const all = [noCve, withCve];

      // A record with a CVE never groups with one that has none, from either side.
      expect(getAffectedProducts(all, noCve).map((v) => v.id)).toEqual(["r1"]);
      expect(getAffectedProducts(all, withCve).map((v) => v.id)).toEqual(["r2"]);
    });

    it("orders ties on component, component version, update level and id, independent of input order", () => {
      const a = make({ id: "1", componentName: "lib-a", version: "1.0", updateLevel: "U1" });
      const b = make({ id: "2", componentName: "lib-a", version: "1.0", updateLevel: "U2" });
      const c = make({ id: "3", componentName: "lib-a", version: "2.0", updateLevel: "U1" });
      const d = make({ id: "4", componentName: "lib-b", version: "1.0", updateLevel: "U1" });
      const e = make({ id: "5", componentName: "lib-b", version: "1.0", updateLevel: "U1" });

      const expected = ["1", "2", "3", "4", "5"];
      expect(getAffectedProducts([e, d, c, b, a], a).map((v) => v.id)).toEqual(expected);
      expect(getAffectedProducts([a, b, c, d, e], a).map((v) => v.id)).toEqual(expected);
    });

    it("falls back to the vulnerability id when the CVE is missing", () => {
      const current = make({ id: "a", cveId: "", vulnerabilityId: "VULN-9" });
      const all = [
        current,
        make({ id: "b", cveId: "", vulnerabilityId: "VULN-9" }),
        make({ id: "c", cveId: "", vulnerabilityId: "VULN-10" }),
      ];

      expect(getAffectedProducts(all, current).map((v) => v.id)).toEqual(["a", "b"]);
    });

    it("does not group unrelated records when neither identifier is present", () => {
      const current = make({ id: "a", cveId: "", vulnerabilityId: "" });
      const all = [current, make({ id: "b", cveId: "", vulnerabilityId: "" })];

      expect(getAffectedProducts(all, current).map((v) => v.id)).toEqual(["a"]);
    });

    it("returns an empty list when there is no current record", () => {
      expect(getAffectedProducts([make({ id: "x" })], undefined)).toEqual([]);
    });
  });

  describe("groupAffectedProducts", () => {
    const make = (overrides: Partial<ProductVulnerability>): ProductVulnerability => ({
      id: "id",
      cveId: "CVE-2026-1",
      vulnerabilityId: "VULN-1",
      severity: { id: "1", label: "High" },
      productName: "Product A",
      productVersion: "1.0.0",
      componentName: "lib-a",
      version: "1.0",
      type: "library",
      useCase: null,
      justification: null,
      resolution: null,
      updateLevel: "U1",
      ...overrides,
    });

    it("puts the components of one product version in one group, in first-seen order", () => {
      const groups = groupAffectedProducts([
        make({ id: "1", productName: "A", productVersion: "1", componentName: "x" }),
        make({ id: "2", productName: "A", productVersion: "2", componentName: "x" }),
        make({ id: "3", productName: "A", productVersion: "1", componentName: "y" }),
        make({ id: "4", productName: "B", productVersion: "1", componentName: "x" }),
      ]);

      expect(groups.map((g) => [g.productName, g.productVersion, g.records.map((r) => r.id)])).toEqual([
        ["A", "1", ["1", "3"]],
        ["A", "2", ["2"]],
        ["B", "1", ["4"]],
      ]);
    });

    it("matches product name and version case-insensitively and ignoring surrounding whitespace", () => {
      const a = make({ id: "1", productName: "WSO2 IS", productVersion: "7.0.0" });
      const b = make({ id: "2", productName: " wso2 is ", productVersion: "7.0.0 " });

      expect(getAffectedProductGroupKey(a)).toBe(getAffectedProductGroupKey(b));
      const groups = groupAffectedProducts([a, b]);
      expect(groups).toHaveLength(1);
      // displays the first record's spelling, trimmed
      expect(groups[0].productName).toBe("WSO2 IS");
    });

    it("keeps different versions of a product, and different products with one version, apart", () => {
      const groups = groupAffectedProducts([
        make({ id: "1", productName: "A", productVersion: "1" }),
        make({ id: "2", productName: "A", productVersion: "2" }),
        make({ id: "3", productName: "B", productVersion: "1" }),
      ]);
      expect(groups).toHaveLength(3);
    });

    it("collects each distinct update level once, in order, and skips blanks", () => {
      const [group] = groupAffectedProducts([
        make({ id: "1", updateLevel: "U2" }),
        make({ id: "2", updateLevel: "U1" }),
        make({ id: "3", updateLevel: "U2" }),
        make({ id: "4", updateLevel: undefined }),
        make({ id: "5", updateLevel: "  " }),
      ]);
      expect(group.updateLevels).toEqual(["U2", "U1"]);
      expect(group.records).toHaveLength(5);
    });

    it("groups records without a product name or version together and returns nothing for no records", () => {
      const groups = groupAffectedProducts([
        make({ id: "1", productName: undefined, productVersion: undefined }),
        make({ id: "2", productName: "", productVersion: "" }),
      ]);
      expect(groups).toHaveLength(1);
      expect(groups[0].productName).toBe("");
      expect(groupAffectedProducts([])).toEqual([]);
    });
  });
});
