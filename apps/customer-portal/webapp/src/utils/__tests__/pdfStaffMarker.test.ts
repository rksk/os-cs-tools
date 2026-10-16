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

import { beforeEach, describe, expect, it, vi } from "vitest";
import { downloadPdfFile } from "@utils/pdf";
import { buildPdfTableBody } from "@utils/pdfStaffMarker";

// Real jsPDF, except save(): it is replaced by a capture of the generated PDF so the
// tests can read what actually went into the file.
const captured = vi.hoisted(() => ({ pdfs: [] as string[], circles: [] as number[][] }));

vi.mock("jspdf", async (importOriginal) => {
  const actual = await importOriginal<typeof import("jspdf")>();
  const RealJsPdf = actual.default;
  function CapturingJsPdf(...args: ConstructorParameters<typeof RealJsPdf>) {
    const doc = new RealJsPdf(...args);
    doc.save = (() => {
      captured.pdfs.push(doc.output());
      return doc;
    }) as typeof doc.save;
    const realCircle = doc.circle.bind(doc);
    doc.circle = ((...circleArgs: Parameters<typeof doc.circle>) => {
      captured.circles.push(circleArgs.slice(0, 3) as number[]);
      return realCircle(...circleArgs);
    }) as typeof doc.circle;
    return doc;
  }
  return { ...actual, default: CapturingJsPdf };
});

describe("buildPdfTableBody", () => {
  it("keeps the name as plain text and notes which cells carry the marker", () => {
    const { body, staffMarkerCells } = buildPdfTableBody([
      ["CHG1", "Danidu Wijendra ⓦ"],
      ["CHG2", "Jane Smith"],
    ]);

    expect(body).toEqual([
      ["CHG1", "Danidu Wijendra"],
      ["CHG2", "Jane Smith"],
    ]);
    expect([...staffMarkerCells]).toEqual(["0:1"]);
  });

  it("identifies marked cells by row and column", () => {
    const { staffMarkerCells } = buildPdfTableBody([
      ["a", "b", "c"],
      ["d", "e ⓦ", "f"],
      ["g ⓦ", "h", "i ⓦ"],
    ]);

    expect([...staffMarkerCells].sort()).toEqual(["1:1", "2:0", "2:2"]);
  });

  it("copes with a cell that is not a string", () => {
    const { body, staffMarkerCells } = buildPdfTableBody([[5 as unknown as string, null as unknown as string]]);

    expect(body).toEqual([["5", ""]]);
    expect(staffMarkerCells.size).toBe(0);
  });

  it("leaves a cell with nothing but the marker empty and unmarked", () => {
    const { body, staffMarkerCells } = buildPdfTableBody([["ⓦ"]]);

    expect(body).toEqual([[""]]);
    expect(staffMarkerCells.size).toBe(0);
  });
});

describe("downloadPdfFile with WSO2 staff names", () => {
  beforeEach(() => {
    captured.pdfs.length = 0;
    captured.circles.length = 0;
  });

  const rows = [
    ["CHG1", "Vimukthi Perera ⓦ"],
    ["CHG2", "Jane Smith"],
    ["CHG3", "Osura Weerasinghe ⓦ"],
  ];

  it("writes each name as plain text instead of letter-spaced UCS-2", () => {
    downloadPdfFile("change-requests.pdf", "change requests — Project", ["Number", "Assigned to"], rows);

    const pdf = captured.pdfs[0];
    expect(pdf).toMatch(/\(Vimukthi Perera\) Tj/);
    expect(pdf).toMatch(/\(Osura Weerasinghe\) Tj/);
    expect(pdf).toContain("(Jane Smith) Tj");
    expect(pdf).not.toContain("\u0000");
  });

  it("draws the circled w once per WSO2 staff name, and nowhere else", () => {
    downloadPdfFile("change-requests.pdf", "change requests — Project", ["Number", "Assigned to"], rows);

    const pdf = captured.pdfs[0];
    expect(pdf.match(/\(w\) Tj/g)).toHaveLength(2);
    // Four Bézier curves per circle.
    expect(pdf.match(/ c\n/g)).toHaveLength(2 * 4);
  });

  it("draws no marker when nobody on the page is WSO2 staff", () => {
    downloadPdfFile("change-requests.pdf", "title", ["Number", "Assigned to"], [["CHG1", "Jane Smith"]]);

    expect(captured.pdfs[0]).not.toContain("(w) Tj");
  });

  it.each(["left", "center", "right"] as const)(
    "keeps the circle inside its cell in a column aligned %s",
    (halign) => {
      downloadPdfFile(
        "change-requests.pdf",
        "title",
        ["Number", "Assigned to"],
        [["CHG1", "Vimukthi Perera ⓦ"]],
        { 0: { cellWidth: 30 }, 1: { cellWidth: 50, halign } },
      );

      // Page margin 14mm + first column 30mm: the cell spans 44-94mm, padded 3mm each side.
      const [centerX, , radius] = captured.circles[0];
      expect(captured.circles).toHaveLength(1);
      expect(centerX - radius).toBeGreaterThanOrEqual(47 - 0.3);
      expect(centerX + radius).toBeLessThanOrEqual(91 + 0.3);
      if (halign === "right") expect(91 - (centerX + radius)).toBeLessThan(0.6);
    },
  );

  it("keeps one circle per staff name, inside its cell, at every column width and name length", () => {
    const names = [
      "Al ⓦ",
      "Danidu Wijendra ⓦ",
      "Praveen Mallikarachchi ⓦ",
      "Luxman Ravindrakumar ⓦ",
      "Supercalifragilisticexpialidocious ⓦ",
    ];
    for (const width of [18, 20, 22, 24, 26, 28, 30, 33, 36, 40, 50, 60]) {
      for (const halign of ["left", "center", "right"] as const) {
        captured.circles.length = 0;
        downloadPdfFile(
          "change-requests.pdf",
          "title",
          ["Number", "Assigned to"],
          names.map((name, index) => [`CHG${index}`, name]),
          { 0: { cellWidth: 30 }, 1: { cellWidth: width, halign } },
        );

        const where = `width ${width}, ${halign}`;
        expect(captured.circles, where).toHaveLength(names.length);
        for (const [centerX, , radius] of captured.circles) {
          // The cell spans 44mm to 44 + width: the circle never leaves it.
          expect(centerX - radius, where).toBeGreaterThanOrEqual(44);
          expect(centerX + radius, where).toBeLessThanOrEqual(44 + width + 0.01);
        }
      }
    }
  });

  it("marks every staff name across a table that runs onto further pages", () => {
    const manyRows = Array.from({ length: 80 }, (_, index) => [
      `CHG${index}`,
      `Engineer ${index} ⓦ`,
    ]);
    downloadPdfFile("change-requests.pdf", "title", ["Number", "Assigned to"], manyRows);

    const pdf = captured.pdfs[0];
    expect(pdf.match(/\/Type \/Page(?!s)/g)?.length).toBeGreaterThan(1);
    expect(pdf.match(/\(w\) Tj/g)).toHaveLength(80);
    expect(captured.circles).toHaveLength(80);
    // A4 landscape is 210mm high: every circle sits on a page, none off the bottom.
    expect(captured.circles.every(([, centerY]) => centerY > 0 && centerY < 210)).toBe(true);
  });

  it("draws a staff name in the same place whether or not other rows carry a marker", () => {
    downloadPdfFile("a.pdf", "title", ["Number", "Assigned to"], [["CHG1", "Jane Smith"]]);
    downloadPdfFile("b.pdf", "title", ["Number", "Assigned to"], [["CHG1", "Jane Smith"], ["CHG2", "Ada Lovelace ⓦ"]]);

    const firstRowText = (pdf: string) => pdf.match(/\(CHG1\) Tj/g)?.length;
    expect(firstRowText(captured.pdfs[0])).toBe(1);
    expect(firstRowText(captured.pdfs[1])).toBe(1);
  });

  it("documents the failure it prevents: jsPDF garbles the whole string around an unencodable character", async () => {
    const { default: JsPdf } = await import("jspdf");
    const doc = new JsPdf();
    doc.text("Vimukthi Perera ⓦ", 10, 10);

    expect(doc.output()).toContain("\u0000");
  });
});
