/** @vitest-environment jsdom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { currentAgentEdge } from "./agent-edge-signal";
import { AgentRail } from "./agentrail";
import { LABELS } from "./agentrail-copy";
import { meFixture } from "./mefixture";

// A mailbox import is the one long run the AI-activity feed cannot carry, so
// the rail reads it off the connections list instead (capture-progress.ts).
// These cases prove that reading reaches every surface the rail owns: the
// orb's state, the ring around it, the line under it, and the lit edge. They
// live apart from agentrail.test.tsx because that file is past the size a test
// file may grow to.

type Connector = components["schemas"]["CaptureConnection"];
type BackfillStatus = components["schemas"]["BackfillStatus"];

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const emptyPage = { has_more: false, next_cursor: null };

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

const IMPORTING: BackfillStatus = {
  state: "running",
  estimated_messages: 1_000,
  counts: { messages_scanned: 420 },
};

type Answers = Readonly<{
  connectors: readonly Connector[];
  running?: readonly unknown[];
  profile?: string;
}>;

// Every read the section makes, answered healthy unless the case says
// otherwise: a model bound, a valid licence, nothing waiting, no run live.
function stubApi(answers: Answers) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const pathname = new URL(request.url).pathname;
      if (pathname.endsWith("/connectors")) {
        return jsonResponse({ data: answers.connectors });
      }
      if (pathname.endsWith("/me/ai-activity")) {
        return jsonResponse({ running: answers.running ?? [], recent: [] });
      }
      if (pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow: { license: ["read"] } }));
      }
      if (pathname.endsWith("/assistant/profile")) {
        return jsonResponse({
          name: "Margince",
          kind: "ai",
          state: answers.profile ?? "configured",
          inference_mode: "cloud",
          providers: [],
        });
      }
      if (pathname.endsWith("/installation/license")) {
        return jsonResponse({
          state: "valid",
          seats_used: 1,
          over_limit: false,
          checked_at: "2026-08-01T09:00:00Z",
        });
      }
      if (pathname.endsWith("/ai/usage")) {
        return jsonResponse({
          days: [],
          budget: { monthly_tokens: 0, spent_tokens: 0, band: "normal" },
        });
      }
      return jsonResponse({ data: [], page: emptyPage });
    }),
  );
}

function mount(answers: Answers) {
  stubApi(answers);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AgentRail route={{ screen: "deals" }} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const block = (container: HTMLElement) => {
  const el = container.querySelector(".arblock");
  if (!el) throw new Error("no .arblock in the rendered tree");
  return el;
};

const ringShare = (container: HTMLElement): string | null =>
  container
    .querySelector(".core-progress-value")
    ?.getAttribute("stroke-dasharray") ?? null;

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a mailbox import", () => {
  it("puts the orb in ingest, draws its share as the ring, and lights the edge", async () => {
    const { container } = mount({ connectors: [mailbox(IMPORTING)] });
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("ingest"),
    );
    expect(ringShare(container)).toBe("42 100");
    expect(currentAgentEdge().reading).toBe(true);
  });

  it("says so under the orb, with the share", async () => {
    const { container } = mount({ connectors: [mailbox(IMPORTING)] });
    await waitFor(() =>
      expect(container.querySelector(".arline")?.textContent).toBe(
        "Importing mail history · 42%",
      ),
    );
  });

  it("names the import without a share when the run has no estimate", async () => {
    const { container } = mount({
      connectors: [
        mailbox({
          state: "queued",
          estimated_messages: null,
          counts: { messages_scanned: 0 },
        }),
      ],
    });
    await waitFor(() =>
      expect(container.querySelector(".arline")?.textContent).toBe(
        "Importing mail history",
      ),
    );
    expect(block(container).getAttribute("data-core-state")).toBe("ingest");
    // No denominator, no ring: a ring at a guessed share is a ring drawn wrong.
    expect(container.querySelector(".core-progress")).toBeNull();
  });

  it("is not reported once the run has settled", async () => {
    const { container } = mount({
      connectors: [
        mailbox({
          state: "done",
          estimated_messages: 1_000,
          counts: { messages_scanned: 1_000 },
        }),
      ],
    });
    // The section's own idle line, once every read has answered.
    await waitFor(() =>
      expect(container.querySelector(".arline")?.textContent).toBe(
        LABELS.allClear,
      ),
    );
    expect(block(container).getAttribute("data-core-state")).toBe("idle");
    expect(container.querySelector(".core-progress")).toBeNull();
    expect(currentAgentEdge().reading).toBe(false);
  });

  it("keeps its ring under a run the feed names, because the import did not stop", async () => {
    const { container } = mount({
      connectors: [mailbox(IMPORTING)],
      running: [
        {
          id: "run-brief",
          kind: "morning_brief",
          state: "running",
          started_at: "2026-08-01T09:00:00Z",
        },
      ],
    });
    // The named occurrence owns the colour and the line...
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("working"),
    );
    // ...and the ring is still the import's.
    expect(ringShare(container)).toBe("42 100");
  });

  it("drops the ring under a fault: a red orb cannot also be going well", async () => {
    const { container } = mount({
      connectors: [mailbox(IMPORTING)],
      profile: "unconfigured",
    });
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("error"),
    );
    expect(container.querySelector(".core-progress")).toBeNull();
  });
});
