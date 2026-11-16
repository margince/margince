// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { parseInline } from "./markdown-inline";

it("reads a destination with one balanced parenthesis pair and surrounding space", () => {
  expect(parseInline("[Docs]( \thttps://example.test/a(b) \t)")).toEqual([
    {
      kind: "link",
      href: "https://example.test/a(b)",
      children: [{ kind: "text", text: "Docs" }],
    },
  ]);
});

it("keeps an incomplete link with a long whitespace destination as text", () => {
  const source = `[Docs](${" ".repeat(50_000)}`;
  expect(parseInline(source)).toEqual([{ kind: "text", text: source }]);
});

it.each([
  "[Docs](https://example.test/a(b)",
  "[Docs](https://example.test/a(b(c)))",
  "[Docs](https://example.test/a b)",
])("keeps malformed destinations as text: %s", (source) => {
  expect(parseInline(source)).toEqual([{ kind: "text", text: source }]);
});
