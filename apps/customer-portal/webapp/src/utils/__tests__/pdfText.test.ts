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
import { hasWso2StaffMarker, toPdfSafeText } from "@utils/pdfText";

describe("toPdfSafeText", () => {
  it("removes the WSO2 staff marker and the space before it (it is drawn separately)", () => {
    expect(toPdfSafeText("Danidu Wijendra ⓦ")).toBe("Danidu Wijendra");
    expect(toPdfSafeText("Ada Lovelace\u00A0ⓦ")).toBe("Ada Lovelace");
  });

  it("leaves Latin-1 text and the cp1252 punctuation jsPDF can draw untouched", () => {
    const text = "José Müller – “quoted” … 5 € • Ørsted ß";
    expect(toPdfSafeText(text)).toBe(text);
  });

  it("returns an empty string for null, undefined and empty input", () => {
    expect(toPdfSafeText(null)).toBe("");
    expect(toPdfSafeText(undefined)).toBe("");
    expect(toPdfSafeText("")).toBe("");
  });

  it("removes zero-width and directional characters", () => {
    expect(toPdfSafeText("Jo\u200Bhn\u200E Doe\uFEFF")).toBe("John Doe");
  });

  it("draws other Unicode spaces as a plain space", () => {
    expect(toPdfSafeText("A\u2009B\u3000C")).toBe("A B C");
  });

  it("folds characters to a plain letter where one exists", () => {
    expect(toPdfSafeText("ń ź ś č")).toBe("n z s c");
    expect(toPdfSafeText("① Ａ")).toBe("1 A");
  });

  it("replaces characters with no encodable form by a single '?'", () => {
    expect(toPdfSafeText("李雷")).toBe("??");
    expect(toPdfSafeText("done 🎉")).toBe("done ?");
    expect(toPdfSafeText("Łukasz")).toBe("?ukasz");
  });

  it("only ever returns characters jsPDF's built-in fonts can encode", () => {
    const out = toPdfSafeText("Danidu ⓦ 李 🎉 Łukasz ✓ “x” – ok");
    const encodable = (char: string) =>
      (char.codePointAt(0) ?? 0) <= 0xff || "“”–".includes(char);
    expect([...out].every(encodable)).toBe(true);
  });
});

describe("hasWso2StaffMarker", () => {
  it("is true only for text that carries the WSO2 staff marker", () => {
    expect(hasWso2StaffMarker("Danidu Wijendra ⓦ")).toBe(true);
    expect(hasWso2StaffMarker("Danidu Wijendra")).toBe(false);
    expect(hasWso2StaffMarker("")).toBe(false);
    expect(hasWso2StaffMarker(null)).toBe(false);
    expect(hasWso2StaffMarker(undefined)).toBe(false);
  });
});

describe("toPdfSafeText never changes text jsPDF could already draw", () => {
  const WIN_ANSI_EXTRAS = [
    338, 339, 352, 353, 376, 381, 382, 402, 710, 732, 8211, 8212, 8216, 8217, 8218,
    8220, 8221, 8222, 8224, 8225, 8226, 8230, 8240, 8249, 8250, 8364, 8482,
  ];

  it("returns every Latin-1 and cp1252 character exactly as it came", () => {
    const codes = [...Array.from({ length: 256 }, (_, code) => code), ...WIN_ANSI_EXTRAS];
    for (const code of codes) {
      const char = String.fromCodePoint(code);
      expect(toPdfSafeText(char), `U+${code.toString(16)}`).toBe(char);
    }
  });

  it("keeps the no-break space and soft hyphen, which affect how jsPDF wraps a line", () => {
    const text = `10${String.fromCodePoint(0xa0)}MB re${String.fromCodePoint(0xad)}start`;
    expect(toPdfSafeText(text)).toBe(text);
  });

  it("returns printable ASCII, line breaks and tabs untouched", () => {
    const text = "CHG0041408 [ACUPROD-798][PROD] U2 Update\tfrom 185 to 189\r\nline 2";
    expect(toPdfSafeText(text)).toBe(text);
  });
});

describe("toPdfSafeText on awkward input", () => {
  const COMBINING_ACUTE = String.fromCodePoint(0x301);

  it("composes a decomposed accent instead of losing the letter", () => {
    expect(toPdfSafeText(`Jose${COMBINING_ACUTE}`)).toBe(`Jos${String.fromCodePoint(0xe9)}`);
    expect(toPdfSafeText(`Jose${COMBINING_ACUTE} Mu${String.fromCodePoint(0x308)}ller`)).toBe(
      `Jos${String.fromCodePoint(0xe9)} M${String.fromCodePoint(0xfc)}ller`,
    );
  });

  it("drops a combining accent that has no precomposed letter, keeping the letter", () => {
    expect(toPdfSafeText(`x${COMBINING_ACUTE}`)).toBe("x");
    expect(toPdfSafeText(`n${COMBINING_ACUTE}`)).toBe("n");
  });

  it("turns Unicode line and paragraph separators into line breaks", () => {
    expect(toPdfSafeText(`a${String.fromCodePoint(0x2028)}b${String.fromCodePoint(0x2029)}c`)).toBe("a\nb\nc");
  });

  it("draws a narrow no-break space (U+202F, as some browsers put before AM/PM) as a plain space", () => {
    expect(toPdfSafeText(`Oct 14, 2026, 12:27${String.fromCodePoint(0x202f)}PM`)).toBe(
      "Oct 14, 2026, 12:27 PM",
    );
  });

  it("does not throw on a value that is not a string", () => {
    expect(toPdfSafeText(5)).toBe("5");
    expect(toPdfSafeText(0)).toBe("0");
    expect(toPdfSafeText(false)).toBe("false");
    expect(hasWso2StaffMarker(5)).toBe(false);
  });

  it("does not throw on a lone surrogate", () => {
    expect(toPdfSafeText(`a${String.fromCharCode(0xd83d)}b`)).toBe("a?b");
  });
});
