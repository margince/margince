// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { collapseSpace } from "./markdown-highlight";

// The server locates a claim's quote under `claims.CollapseSpace`, which folds
// on Go's `unicode.IsSpace`. This has to fold the SAME set or a quote carrying
// a character the two disagree about normalises to two different strings: the
// server finds its match, the viewer does not, and the citation opens the right
// document with nothing marked in it.
//
// `backend/gates/frontendcollapsespace_test.go` holds the whole set against
// `unicode.IsSpace` itself and fails in both directions. These are the two
// characters `\\s` actually got wrong, kept here as the readable statement of
// what that gate is for.
describe("the viewer folds the whitespace the server folds", () => {
  it("folds the ordinary run, collapsed and trimmed", () => {
    expect(collapseSpace("  kept for\n  400   days \t")).toBe(
      "kept for 400 days",
    );
  });

  // NEL arrives in mainframe and EBCDIC exports, where it ends a line.
  it("folds U+0085 NEL, which Go folds and /s does not", () => {
    expect(collapseSpace("kept for\u0085400 days")).toBe("kept for 400 days");
  });

  it("folds the non-breaking space, as both sides do", () => {
    expect(collapseSpace("kept for\u00a0400 days")).toBe("kept for 400 days");
  });

  // A BOM heads most Windows-authored files, so it lands on the first line of
  // an uploaded document — the one a handbook's title lives on. The server
  // leaves it in the text, so folding it here would make the viewer match a
  // span the server never checked the quote against.
  it("leaves U+FEFF alone, because the server does", () => {
    expect(collapseSpace("\ufeffMessages are kept")).toBe(
      "\ufeffMessages are kept",
    );
  });
});
