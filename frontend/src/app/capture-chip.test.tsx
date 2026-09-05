/** @vitest-environment jsdom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { CaptureChip } from "./capture-chip";
import { useDrawsImportRun } from "./import-onscreen";

// The chip is the import's gauge, bottom-centre of the content: present for
// exactly as long as mail is arriving, carrying the server's own counts, and
// leading to the card that holds the run in full — except where a surface on
// screen already draws that run, and except where there is nowhere to lead.

type Connector = components["schemas"]["CaptureConnection"];
type BackfillStatus = components["schemas"]["BackfillStatus"];

function mailbox(backfill?: BackfillStatus): Connector {
  return {
    id: "018f3a1b-0000-7000-8000-0000000000c1",
    provider: "gmail",
    status: "connected",
    scopes: [],
    account_label: "ada@acme.test",
    backfill,
  };
}

function stubConnectors(connectors: readonly Connector[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ data: connectors }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
}

// A stand-in for the connections card and onboarding's backread step: any
// surface that declares it is drawing the run in full.
function RunPanel() {
  useDrawsImportRun(true);
  return <p>the run, in full</p>;
}

function mount(
  connectors: readonly Connector[],
  options?: Readonly<{ linked?: boolean; drawnOnScreen?: boolean }>,
) {
  stubConnectors(connectors);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {options?.drawnOnScreen === true && <RunPanel />}
        <CaptureChip linked={options?.linked ?? true} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("CaptureChip", () => {
  it("draws the import with its share, the counts and a way to the run", async () => {
    mount([
      mailbox({
        state: "running",
        estimated_messages: 2_900,
        counts: { messages_scanned: 1_218 },
      }),
    ]);
    const link = await screen.findByRole("link", {
      name: "Importing mail history. 42% · 1,218 of 2,900 messages. Open the import",
    });
    expect(link.getAttribute("href")).toBe("#/settings/connections");
    expect(screen.getByRole("status").textContent).toContain(
      "42% · 1,218 of 2,900 messages",
    );
    // The chip's own Core wears the import's ring at the same share.
    expect(
      link
        .querySelector(".core-progress-value")
        ?.getAttribute("stroke-dasharray"),
    ).toBe("42 100");
    expect(link.querySelector(".core")?.getAttribute("data-core-state")).toBe(
      "ingest",
    );
  });

  it("counts rather than guesses when the run has no estimate", async () => {
    mount([
      mailbox({
        state: "running",
        estimated_messages: null,
        counts: { messages_scanned: 37 },
      }),
    ]);
    const link = await screen.findByRole("link");
    expect(link.textContent).toContain("37 messages so far");
    expect(link.querySelector(".core-progress")).toBeNull();
  });

  it("is absent while nothing is importing", async () => {
    const { container } = mount([
      mailbox({ state: "done", estimated_messages: 10 }),
    ]);
    // Wait for the read to answer, then assert on the settled nothing.
    await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled());
    await waitFor(() => expect(container.innerHTML).toBe(""));
  });

  it("stands down where a surface already draws the run", async () => {
    const { container } = mount(
      [
        mailbox({
          state: "running",
          estimated_messages: 100,
          counts: { messages_scanned: 1 },
        }),
      ],
      { drawnOnScreen: true },
    );
    await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled());
    expect(screen.getByText("the run, in full")).toBeTruthy();
    await waitFor(() =>
      expect(container.querySelector(".capturechip")).toBeNull(),
    );
  });

  it("gauges without leading anywhere where the route carries no navigation", async () => {
    mount(
      [
        mailbox({
          state: "running",
          estimated_messages: 2_900,
          counts: { messages_scanned: 1_218 },
        }),
      ],
      { linked: false },
    );
    const status = await screen.findByRole("status");
    expect(status.textContent).toContain("42% · 1,218 of 2,900 messages");
    expect(status.querySelector("a")).toBeNull();
  });
});
