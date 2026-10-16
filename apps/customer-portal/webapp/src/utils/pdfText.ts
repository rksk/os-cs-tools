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
 * WSO2 staff display names carry a trailing "ⓦ" (U+24E6) marker, e.g. "Ada Lovelace ⓦ",
 * which tells a customer they are dealing with a WSO2 engineer. No PDF font we ship can
 * draw it, so {@link toPdfSafeText} removes it from the text and the table exports draw
 * it themselves (see pdfStaffMarker.ts).
 */
const WSO2_STAFF_MARKER = /\s*\u24E6/g;

/**
 * Returns whether the text carries the WSO2 staff marker ("ⓦ").
 *
 * @param value - Text destined for a PDF cell.
 * @returns {boolean} True when the marker is present.
 */
export function hasWso2StaffMarker(value: unknown): boolean {
  return typeof value === "string" && value.includes("\u24E6");
}

/** Printable ASCII plus line breaks and tabs: nothing in it can need changing. */
const PLAIN_TEXT = /^[\x20-\x7E\n\r\t]*$/;

/** Any Unicode space (thin, ideographic, ...). */
const UNICODE_SPACE = /^\p{Zs}$/u;

/** Zero-width and directional format characters, and emoji variation selectors. */
const INVISIBLE = /^[\p{Cf}\uFE00-\uFE0F]$/u;

/** A combining accent with no precomposed form: nothing to attach to once the letter is gone. */
const COMBINING_MARK = /^\p{M}$/u;

/** Combining accents left behind once a letter is decomposed (the "´" of "ń"). */
const COMBINING_MARKS = /\p{M}/gu;

/**
 * Beyond U+00FF, the only characters jsPDF's built-in fonts can encode: the cp1252
 * ("WinAnsi") extras it remaps (smart quotes, dashes, bullet, ellipsis, euro, ...).
 * See the WinAnsiEncoding table in jspdf.
 */
const WIN_ANSI_EXTRAS: ReadonlySet<number> = new Set([
  338, 339, 352, 353, 376, 381, 382, 402, 710, 732, 8211, 8212, 8216, 8217, 8218,
  8220, 8221, 8222, 8224, 8225, 8226, 8230, 8240, 8249, 8250, 8364, 8482,
]);

function isEncodable(char: string): boolean {
  const code = char.codePointAt(0) ?? 0;
  return code <= 0xff || WIN_ANSI_EXTRAS.has(code);
}

/** The encodable letters a character folds to ("ń" -> "n", "①" -> "1"), or "" when none. */
function foldToEncodable(char: string): string {
  const folded = char.normalize("NFKD").replace(COMBINING_MARKS, "");
  return [...folded].every(isEncodable) ? folded : "";
}

/** What an unencodable character is drawn as. */
function replacementFor(char: string): string {
  if (UNICODE_SPACE.test(char)) return " ";
  if (char === "\u2028" || char === "\u2029") return "\n";
  if (INVISIBLE.test(char) || COMBINING_MARK.test(char)) return "";
  return foldToEncodable(char) || "?";
}

/**
 * Makes text safe to hand to jsPDF's built-in (Helvetica) fonts.
 *
 * jsPDF cannot draw characters outside Latin-1/cp1252 with those fonts, and a single one
 * in a string makes it write the WHOLE string as UCS-2: viewers then show the text
 * letter-spaced and clipped, with junk (e.g. "$æ" for "ⓦ") at the end. Characters that
 * cannot be drawn are therefore removed (the WSO2 staff marker, zero-width characters),
 * folded to a plain letter where one exists, or replaced with "?" so a gap is visible
 * rather than silent.
 *
 * Text jsPDF could already draw is returned exactly as it came: only characters it cannot
 * encode are ever touched, so output that was correct before is unchanged.
 *
 * @param value - Text destined for a PDF cell, title or line.
 * @returns {string} The text with every character encodable by jsPDF's built-in fonts.
 */
export function toPdfSafeText(value: unknown): string {
  if (value == null || value === "") return "";
  const text = typeof value === "string" ? value : String(value);
  if (PLAIN_TEXT.test(text)) return text;

  // NFC first, so "e" + a combining accent becomes the "é" jsPDF can draw.
  let safe = "";
  for (const char of text.replace(WSO2_STAFF_MARKER, "").normalize("NFC")) {
    safe += isEncodable(char) ? char : replacementFor(char);
  }
  return safe;
}
