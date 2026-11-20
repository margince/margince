// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { looksLikeEmail } from "./emailshape";

describe("an email address a form lets through", () => {
  it.each([
    "o'brien@example.com",
    "asker@localhost",
    "first.last+tag@sub.example.org",
    " padded@example.com ",
  ])("takes %s, which the server accepts", (address) => {
    expect(looksLikeEmail(address)).toBe(true);
  });

  it.each([
    "not-an-email",
    "@example.com",
    "asker@",
    "two@at@example.com",
    "with space@example.com",
    "",
  ])("refuses %j", (text) => {
    expect(looksLikeEmail(text)).toBe(false);
  });
});
