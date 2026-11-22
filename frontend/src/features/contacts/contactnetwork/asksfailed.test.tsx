/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../../i18n/en";
import { anchor, CONTACT, renderTab } from "./network.testkit";

// An introduction list that could not be read keeps saying so, with its Retry,
// for as long as the read runs again.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const alone = () => ({
  contact_id: CONTACT,
  nodes: [anchor],
  edges: [],
  routes: [],
  groups_omitted: [],
});
const FAILED = en["contact.intro.asksFailed"];
const RETRY = en["common.retry"];

describe("an introduction list that could not be read", () => {
  it("keeps its Retry", async () => {
    renderTab(alone(), "en", {}, { asksFail: true });

    expect(await screen.findByText(FAILED)).toBeTruthy();
    expect(screen.getByRole("button", { name: RETRY })).toBeTruthy();
  });

  it("stays failed while the read runs again", async () => {
    renderTab(alone(), "en", {}, { asksFail: true });
    expect(await screen.findByText(FAILED)).toBeTruthy();
    vi.mocked(fetch).mockImplementation(() => new Promise<Response>(() => {}));

    await userEvent.click(screen.getByRole("button", { name: RETRY }));
    expect(screen.getByText(FAILED)).toBeTruthy();
    expect(screen.getByText(en["contact.intro.handoffUnread"])).toBeTruthy();
  });
});
