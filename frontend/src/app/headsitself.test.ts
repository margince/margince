// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { headsItself } from "./pagemeta";
import { parseHash } from "./router";

// Which pages print their own h1 is asked of the ROUTE: under one screen,
// Filters and views, the library is named by the shell and every focused page
// names itself. The self-headed census (self-headed-coverage.test.ts) reads
// screens, not addresses, so this is the table that holds the address half.

describe("a page that heads itself", () => {
  it.each([
    "#/filters/contacts",
    "#/filters/contacts/v1",
    "#/filters/list/L1",
    "#/lists/L1",
    // A screen in the self-headed set is still one, whatever its address.
    "#/home",
    "#/deals",
  ])("%s prints its own heading", (hash) => {
    expect(headsItself(parseHash(hash))).toBe(true);
  });

  it.each([
    "#/filters",
    "#/filters/views",
    "#/filters/lists",
    // An address the library does not recognise opens the library.
    "#/filters/widgets",
    "#/analytics",
  ])("%s is named by the shell", (hash) => {
    expect(headsItself(parseHash(hash))).toBe(false);
  });
});
