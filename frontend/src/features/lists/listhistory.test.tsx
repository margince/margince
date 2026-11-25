// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../../i18n/en";
import type { ListHistoryEntry } from "../../lib/lists.queries";
import {
  listsMe,
  liveHistory,
  liveList,
  liveVocabulary,
  revisionHistory,
} from "../../testing/lists.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ListHistoryPanel } from "./listhistory";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function panel(rows: readonly ListHistoryEntry[]) {
  installFetchStub({
    "GET /me": listsMe(true),
    "GET /filters/vocabulary": () => jsonResponse(liveVocabulary),
    [`GET /lists/${liveList.id}/history`]: () =>
      jsonResponse({ data: rows, page: { has_more: false } }),
  });
  render(
    <StoryProviders>
      <ListHistoryPanel list={liveList} />
    </StoryProviders>,
  );
}

describe("ListHistoryPanel", () => {
  it("names the record that joined or left, opening it", async () => {
    panel(liveHistory);
    expect(
      await screen.findByRole("button", { name: "MiTek" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Krones" })).toBeInTheDocument();
  });

  it("dates a change in the short form with its time", async () => {
    panel(liveHistory);
    expect(
      await screen.findByText(/^29 Sept? 2026, 08:15$/),
    ).toBeInTheDocument();
  });

  it("says what a revision changed, the filter before and after in words", async () => {
    panel(revisionHistory);
    expect(
      await screen.findByText(
        "Renamed it from Quiet manufacturers to Quiet German manufacturers",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(en["lists.history.refiltered"]),
    ).toBeInTheDocument();
    const before = await screen.findByText(/^Before: /);
    const after = screen.getByText(/^After: /);
    expect(before.textContent).toMatch(/industry/i);
    expect(after.textContent).not.toBe(before.textContent);
    expect(screen.getByText(en["lists.history.created"])).toBeInTheDocument();
  });

  it("calls an agent's change an agent's, never Someone", async () => {
    panel(revisionHistory);
    expect(
      await screen.findByText(en["trust.agentUnnamed"]),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(en["lists.history.someone"]),
    ).not.toBeInTheDocument();
  });
});
