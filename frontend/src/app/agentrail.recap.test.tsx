/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { AgentRail } from "./agentrail";
import { LABELS } from "./agentrail-copy";
import { meFixture } from "./mefixture";

// The panel's recap: what the agent has done, and WHICH RECORD it did it on.
//
// The claim under every case here is that the recap reads the AI-activity feed
// rather than the model-call trace. The trace is telemetry and carries no
// subject by design (`Call.Subject` never reaches ai_call), so a recap drawn
// from it could only ever report that something happened; the occurrence is the
// half that knows what the work was about, and the name it carries is the way
// back to the account.
//
// They live apart from agentrail.test.tsx because that file is at the size a
// test file may grow to.

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

const emptyPage = { has_more: false, next_cursor: null };

type AiActivityItem = components["schemas"]["AiActivityItem"];

/** A website read that finished, naming the company it was about: the label is
 *  the name the source knew when it emitted, and the type and id are what turn
 *  that name into a way back to the record. */
const SETTLED_SITE_READ: AiActivityItem = {
  id: "019f7e65-0000-7000-8000-0000000000a1",
  kind: "site_read",
  state: "done",
  started_at: "2026-08-01T09:00:00Z",
  finished_at: "2026-08-01T09:01:00Z",
  subject_label: "Acme GmbH",
  subject_type: "company",
  subject_id: "019f7e65-0000-7000-8000-0000000000b2",
};

/**
 * Every read the section makes, answered with the least the panel needs, and
 * the feed answered by the case itself.
 *
 * The seat holds nothing: the recap is a rep's surface, so a case that granted
 * `ai_diagnostics:read` would be proving the recap on the one seat that also
 * gets the runtime row — and the rows this file is about would be the only
 * thing an ordinary seat could not see.
 */
function stubRail(feed: () => Response | Promise<Response>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      if (pathname.endsWith("/me/ai-activity")) {
        return feed();
      }
      if (pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow: {} }));
      }
      if (pathname.endsWith("/assistant/profile")) {
        return jsonResponse({ state: "configured" });
      }
      if (pathname.endsWith("/connectors")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse({ data: [], page: emptyPage });
    }),
  );
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <AgentRail route={{ screen: "companies" }} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

/** The panel is portalled to the body once open, never inside the container. */
const panel = () => {
  const el = document.querySelector(".arloose");
  if (!el) throw new Error("no .arloose panel on the document");
  return el;
};

async function openPanel(container: HTMLElement) {
  const user = userEvent.setup();
  const trigger = container.querySelector(".arhit");
  if (!trigger) throw new Error("no .arhit trigger in the rendered tree");
  await user.click(trigger);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the agent panel's recap", () => {
  // The case the recap exists for: the record's name is IN the sentence, and it
  // is the way back to the record rather than a word in it.
  it("names the record a settled run was about, and links to it", async () => {
    stubRail(() =>
      jsonResponse({ running: [], recent: [SETTLED_SITE_READ], faults: [] }),
    );
    const { container } = mount();
    await openPanel(container);
    const row = await waitFor(() => {
      const found = panel().querySelector(".aritem:not(.arempty)");
      if (!found) throw new Error("no recap row on the panel yet");
      return found as HTMLElement;
    });
    expect(row.textContent).toContain("I've read the");
    expect(
      within(row).getByRole("link", { name: "Acme GmbH" }).getAttribute("href"),
    ).toBe(`#/companies/${SETTLED_SITE_READ.subject_id}`);
    // The kind is an invocation-site token, and a recap that leaked one would
    // say something happened and nothing about what.
    expect(panel().textContent).not.toContain("site_read");
  });

  // A read that has not answered is ABSENT, never a zero: the empty sentence
  // claims a day somebody looked at, and nobody has.
  it("draws neither a row nor an empty line while the feed read is in flight", async () => {
    stubRail(() => new Promise<Response>(() => {}));
    const { container } = mount();
    await openPanel(container);
    await waitFor(() => expect(panel().textContent).toContain(LABELS.recap));
    expect(panel().querySelector(".aritem")).toBeNull();
  });

  // The server reports every AI task, because a task that reports nothing is AI
  // work the product performed and then denied; what a reader is SHOWN is the
  // client's decision. A background sweep the copy map does not narrate draws no
  // row rather than an invented sentence — it stays in the full log, which is
  // what the heading links to — and a day of nothing else earns the sentence
  // that says so.
  it("says nothing has finished today for a day the rail narrates none of", async () => {
    stubRail(() =>
      jsonResponse({
        running: [],
        recent: [{ ...SETTLED_SITE_READ, kind: "capture_classify" }],
        faults: [],
      }),
    );
    const { container } = mount();
    await openPanel(container);
    await waitFor(() =>
      expect(panel().querySelector(".arempty")?.textContent).toBe(
        LABELS.nothingToday,
      ),
    );
    expect(panel().querySelector(".aritem:not(.arempty)")).toBeNull();
    expect(panel().textContent).not.toContain("capture_classify");
  });
});
