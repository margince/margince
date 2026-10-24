/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  anchor,
  CONTACT,
  type ContactGraph,
  renderTab,
} from "./network.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a colleague on the threads with no mailbox", () => {
  const graph: ContactGraph = {
    contact_id: CONTACT,
    nodes: [anchor],
    edges: [],
    routes: [],
  } as ContactGraph;

  it("is named in a notice that later mail may be missing", async () => {
    renderTab(
      graph,
      "en",
      {},
      {
        captureGaps: [
          {
            user_id: "u-1",
            display_name: "Ina Invited",
            invited: true,
            last_on_thread_at: "2026-10-01T10:00:00Z",
          },
        ],
      },
    );
    expect(
      await screen.findByText(
        /Ina Invited \(has not signed in yet\).*later mail may be missing/,
      ),
    ).toBeTruthy();
  });

  it("draws no notice when every colleague on the threads is captured", async () => {
    const { seatAnswered } = renderTab(graph);
    await seatAnswered;
    await waitFor(() =>
      expect(screen.queryByText(/later mail may be missing/)).toBeNull(),
    );
  });
});
