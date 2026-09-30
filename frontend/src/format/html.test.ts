// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { escapeHtml } from "./html";

it("escapes what could close a tag or an attribute, and the escape itself", () => {
  expect(escapeHtml(`<a href="x">Tom & Jerry</a>`)).toBe(
    "&lt;a href=&quot;x&quot;&gt;Tom &amp; Jerry&lt;/a&gt;",
  );
});

it("leaves every other character as it was, curly quotes and all", () => {
  expect(escapeHtml("Margince lädt wieder, sobald „alles“ zurück ist.")).toBe(
    "Margince lädt wieder, sobald „alles“ zurück ist.",
  );
});
