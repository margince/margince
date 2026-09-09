/** @vitest-environment jsdom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { AgentRail } from "./agentrail";
import { meFixture } from "./mefixture";

// AC-shell-8, on the surface that carries it now.
//
// The claim is a promise about what the agent can REACH — row scope bounds it
// on the server — and it is the one line on the panel that no read produces.
// It has been deleted once already, by a surface rewrite that moved the agent
// from a floating button to this panel and took the sentence with it, leaving
// the guarantee true and unstated. Nothing went red, which is why it stayed
// gone. These assertions are the thing that goes red.
//
// The AC proper lives in `frontend/e2e/ac.spec.ts` beside the other shell ACs,
// over the shipped surface. This is its fast half: it runs on every PR, where
// the e2e lane does not, so the deletion is caught in the diff that makes it.
//
// They live apart from agentrail.test.tsx because that file is at the size a
// test file may grow to.

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// Every read the section makes, answered with the least the panel needs: the
// claim is not a reading, so no answer here should be able to remove it.
function stubApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const pathname = new URL(request.url).pathname;
      if (pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow: {} }));
      }
      if (pathname.endsWith("/me/ai-activity")) {
        return jsonResponse({ running: [], recent: [] });
      }
      if (pathname.endsWith("/connectors")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse({
        data: [],
        page: { has_more: false, next_cursor: null },
      });
    }),
  );
}

async function openPanel(locale: "en" | "de") {
  stubApi();
  const user = userEvent.setup();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { container } = render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <AgentRail route={{ screen: "deals" }} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const trigger = container.querySelector(".arhit");
  if (!trigger) throw new Error("no .arhit trigger in the rendered tree");
  await user.click(trigger);
  const opened = document.querySelector(".arpanel");
  if (!opened) throw new Error("no .arpanel on the document after the click");
  return opened;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("AC-shell-8: the agent panel states what the agent can reach", () => {
  it("carries the scope claim", async () => {
    const opened = await openPanel("en");
    await waitFor(() =>
      expect(opened.textContent).toContain(en["shell.agent.scope"]),
    );
  });

  // A promise a reader cannot read is not a promise. The surface it replaced
  // was English-only in its last weeks, and the sentence that states a
  // guarantee is the last one that may be.
  it("states it in the reader's own language", async () => {
    const opened = await openPanel("de");
    await waitFor(() =>
      expect(opened.textContent).toContain(de["shell.agent.scope"]),
    );
    expect(de["shell.agent.scope"]).not.toBe(en["shell.agent.scope"]);
  });
});
