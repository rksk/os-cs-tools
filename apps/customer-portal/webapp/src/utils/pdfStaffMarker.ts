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

import type { jsPDF } from "jspdf";
import type { CellHookData } from "jspdf-autotable";
import { hasWso2StaffMarker, toPdfSafeText } from "@utils/pdfText";

/**
 * Names of WSO2 staff end in "ⓦ", which is how a customer tells an engineer from a
 * colleague. jsPDF's built-in fonts cannot draw that character, so the PDF exports keep
 * the name as plain text, reserve room after it through the cell's right padding, and
 * draw a circled "w" there (see {@link wso2StaffMarkerHooks}).
 *
 * All sizes are in em, i.e. relative to the font size of the cell the name is in.
 */
const GAP_EM = 0.28;
const DIAMETER_EM = 0.75;
const RADIUS_EM = DIAMETER_EM / 2;
/** How far the circle's centre sits above the text baseline. */
const CENTER_ABOVE_BASELINE_EM = 0.33;
const OUTLINE_WIDTH_EM = 0.06;
/** Size of the "w" inside the circle, relative to the surrounding text. */
const LETTER_SIZE_FACTOR = 0.65;
/** Half the x-height of Helvetica Bold, in em of the "w": centres the letter on the circle. */
const LETTER_HALF_X_HEIGHT_EM = 0.26;
/** autotable lays text lines out with this line height factor (see autoTableText). */
const BASELINE_OFFSET_FACTOR = 2 - 1.15;

const cellKey = (rowIndex: number, columnIndex: number): string =>
  `${rowIndex}:${columnIndex}`;

export type PdfTableBody = {
  /** Table body as autotable should lay it out: every cell made safe for jsPDF's fonts. */
  body: string[][];
  /** "row:column" of the body cells that belong to WSO2 staff. */
  staffMarkerCells: ReadonlySet<string>;
};

/**
 * Prepares table rows for a jsPDF autotable: every cell goes through
 * {@link toPdfSafeText}, and the cells that carried the WSO2 staff marker are noted so
 * {@link wso2StaffMarkerHooks} can make room for, and draw, the circled "w".
 *
 * @param rows - Table data rows.
 * @returns {PdfTableBody} The body to hand to autotable and the cells to mark.
 */
export function buildPdfTableBody(rows: string[][]): PdfTableBody {
  const staffMarkerCells = new Set<string>();
  const body = rows.map((row, rowIndex) =>
    row.map((cell, columnIndex) => {
      const text = toPdfSafeText(cell);
      if (text && hasWso2StaffMarker(cell)) {
        staffMarkerCells.add(cellKey(rowIndex, columnIndex));
      }
      return text;
    }),
  );
  return { body, staffMarkerCells };
}

/**
 * Builds the autotable hooks that give a WSO2 staff name its circled "w" (the "ⓦ" of the
 * name): `didParseCell` widens the cell's right padding by the room the circle needs, so
 * the layout (wrapping, column widths) accounts for it, and `didDrawCell` draws the circle
 * after the last line of the name. Spread both into the autotable options.
 *
 * @param staffMarkerCells - Cells to mark, from {@link buildPdfTableBody}.
 * @returns The `didParseCell` and `didDrawCell` hooks.
 */
export function wso2StaffMarkerHooks(staffMarkerCells: ReadonlySet<string>): {
  didParseCell: (data: CellHookData) => void;
  didDrawCell: (data: CellHookData) => void;
} {
  const isMarked = (data: CellHookData): boolean =>
    data.section === "body" &&
    staffMarkerCells.has(cellKey(data.row.index, data.column.index));

  return {
    didParseCell: (data) => {
      if (!isMarked(data)) return;
      const { cell } = data;
      const fontSize = cell.styles.fontSize / (data.doc as jsPDF).internal.scaleFactor;
      cell.styles.cellPadding = {
        top: cell.padding("top"),
        bottom: cell.padding("bottom"),
        left: cell.padding("left"),
        right: cell.padding("right") + (GAP_EM + DIAMETER_EM) * fontSize,
      };
    },

    didDrawCell: (data) => {
      if (!isMarked(data)) return;
      const doc = data.doc as jsPDF;
      const { cell } = data;
      const lines = cell.text;
      if (lines.length === 0) return;

      const previous = {
        font: doc.getFont(),
        fontSize: doc.getFontSize(),
        lineWidth: doc.getLineWidth(),
        drawColor: doc.getDrawColor(),
        textColor: doc.getTextColor(),
      };

      // Measure with the cell's own font so the circle follows the text it belongs to.
      doc.setFont(cell.styles.font, cell.styles.fontStyle);
      doc.setFontSize(cell.styles.fontSize);

      const fontSize = doc.getFontSize() / doc.internal.scaleFactor;
      const lineHeight = fontSize * doc.getLineHeightFactor();

      // Baseline of the last line, exactly as autotable places it (autoTableText).
      const textPos = cell.getTextPos();
      let firstBaseline = textPos.y + fontSize * BASELINE_OFFSET_FACTOR;
      if (cell.styles.valign === "middle") firstBaseline -= (lines.length / 2) * lineHeight;
      else if (cell.styles.valign === "bottom") firstBaseline -= lines.length * lineHeight;
      const lastBaseline = firstBaseline + (lines.length - 1) * lineHeight;

      // Where the last line starts, as autotable aligns it.
      const lastLine = lines[lines.length - 1];
      const lastLineWidth = doc.getTextWidth(lastLine);
      let lineLeft = textPos.x;
      if (cell.styles.halign === "right") lineLeft -= lastLineWidth;
      else if (cell.styles.halign === "center") lineLeft -= lastLineWidth / 2;

      const centerX = lineLeft + lastLineWidth + (GAP_EM + RADIUS_EM) * fontSize;
      const centerY = lastBaseline - CENTER_ABOVE_BASELINE_EM * fontSize;

      doc.setDrawColor(previous.textColor);
      doc.setLineWidth(OUTLINE_WIDTH_EM * fontSize);
      doc.circle(centerX, centerY, RADIUS_EM * fontSize, "S");

      // The "w": bold, sized to sit inside the circle, centred on its x-height.
      const letterSize = fontSize * LETTER_SIZE_FACTOR;
      doc.setFont("helvetica", "bold");
      doc.setFontSize(doc.getFontSize() * LETTER_SIZE_FACTOR);
      doc.setTextColor(previous.textColor);
      doc.text("w", centerX, centerY + letterSize * LETTER_HALF_X_HEIGHT_EM, { align: "center" });

      doc.setFont(previous.font.fontName, previous.font.fontStyle);
      doc.setFontSize(previous.fontSize);
      doc.setLineWidth(previous.lineWidth);
      doc.setDrawColor(previous.drawColor);
      doc.setTextColor(previous.textColor);
    },
  };
}
