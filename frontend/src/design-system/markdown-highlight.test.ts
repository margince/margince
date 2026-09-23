// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { collapseSpace, markQuote } from "./markdown-highlight";
import type { Block } from "./markdown-parse";

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

// The needle and the HAYSTACK are two separate collapse sites — one builds the
// quote, the other walks the document — and a set fixed at only one of them is
// not fixed. `collapseSpace` alone cannot see that: these go through
// `markQuote`, which is where both sides meet.
describe("a quote marks the document whichever side folds the character", () => {
  const paragraph = (text: string): Block[] => [
    {
      kind: "paragraph",
      line: 1,
      endLine: 1,
      run: { nodes: [{ kind: "text", text }] },
    },
  ];

  it("marks across a U+0085 NEL sitting in the document", () => {
    expect(
      markQuote(
        paragraph("Messages are kept for\u0085400 days."),
        "kept for 400 days",
      ),
    ).toBe(true);
  });

  it("marks a quote whose document opens with a BOM", () => {
    expect(
      markQuote(
        paragraph("\ufeffMessages are kept for 400 days."),
        "\ufeffMessages are kept",
      ),
    ).toBe(true);
  });
});
