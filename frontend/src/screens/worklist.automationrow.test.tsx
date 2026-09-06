// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

// A failed rule says where to go and look at it.
//
// The row named a broken automation — "Notify sales on a new lead", failed —
// and carried no address at all. Not the run, not the rule, not the screen
// either lives on. A reader was told something was wrong and left to go and
// find it, which on a queue whose whole promise is "here is your work" is the
// row doing the opposite of its job.
//
// Fixing the RULE is still the automations page's job, and this row does not
// pretend otherwise: it carries no Investigate verb and names no rule editor.
//
// Running the failed FIRING again is a different act, and the row does perform
// it. Retry used to be declined here on the grounds that a re-run would have to
// replay an event the bus drops after about three days, so the button would
// silently do nothing on day four. That reasoning does not describe the retry
// that exists: it rebuilds the event from `event_outbox`, a table nothing
// prunes short of a workspace reset, and when the envelope genuinely is not
// there it REFUSES in words rather than doing nothing.
//
// The server decides which firings carry the verb. A `blocked` firing does not,
// because blocked is the permission gate having refused it on purpose.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function aFailedRule(over = {}) {
  return row({
    id: "01a05500-0000-7000-8000-0000000000a1",
    source: "automation_run",
    category: "system",
    title: "Notify sales on a new lead",
    band: "keep_momentum",
    destination: "system_health",
    actions: [],
    ...over,
  });
}

function linksIn(container: HTMLElement): (string | null)[] {
  return [...container.querySelectorAll(".worklist-list li a")].map((link) =>
    link.getAttribute("href"),
  );
}

describe("a failed automation reaches the page that owns it", () => {
  it("links the row to the automations page", async () => {
    stub(
      day({
        queue: [aFailedRule()],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    const { container } = renderWorklist();

    await screen.findByText(/Notify sales on a new lead/);
    // The ADDRESS, not merely that a link exists: a row pointing at the wrong
    // screen renders exactly like one pointing at the right screen.
    expect(linksIn(container)).toContain("#/settings/models");
  });

  it("gives the same address to the AI work a rule set off", async () => {
    stub(
      day({
        queue: [
          aFailedRule({
            id: "01a05500-0000-7000-8000-0000000000a2",
            source: "ai_work_health",
            title: "A drafting run did not finish",
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    const { container } = renderWorklist();

    await screen.findByText(/A drafting run did not finish/);
    expect(linksIn(container)).toContain("#/settings/models");
  });

  // The queue can send a reader somewhere; it cannot fix a rule. A button here
  // would promise a repair this surface does not perform — and Retry
  // specifically would promise one the SERVER cannot perform either.
  it("offers no verb when the server sent none", async () => {
    stub(
      day({
        queue: [aFailedRule()],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    await screen.findByText(/Notify sales on a new lead/);
    expect(screen.queryByRole("button", { name: /run it again/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /Investigate/i })).toBeNull();
  });

  it("runs a failed firing again, and says the rule is running", async () => {
    stub(
      day({
        queue: [aFailedRule({ actions: ["retry"] })],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
      { retried: true },
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", { name: /run it again/i }),
    );
    // findByText throws when the text never arrives, so reaching this line is
    // the assertion; the null check states it rather than leaving it implied.
    expect(await screen.findByText(/Running the rule again/i)).not.toBeNull();
  });

  it("says WHY a refused retry did nothing, rather than falling silent", async () => {
    // The refusal arrives as an ordinary 200. A row that swallowed it would
    // leave the reader pressing a button that changes nothing on screen, which
    // is the shape of a control that appears broken.
    stub(
      day({
        queue: [aFailedRule({ actions: ["retry"] })],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
      { retried: false, refusal: "trigger_event_unavailable" },
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", { name: /run it again/i }),
    );
    expect(
      await screen.findByText(/event behind this firing is gone/i),
    ).not.toBeNull();
  });
});

// renderUnderAToastRegion draws the screen the way the shell draws it.
//
// Both answers a retry can give — started, and refused with a reason — are
// spoken in a toast, and `renderWorklist` mounts no region: the testkit is
// production-shaped for the conformance gate, which allows exactly one
// ToastProvider and one ToastRegion and names main.tsx as their home. So the
// region is mounted here, in a test file the gate does not scan, the same way
// worklist.taskdone.test.tsx does it.
//
// Without it `toast.show` renders nothing and a retry test would assert only
// that the button was pressable — passing over a control that answers nothing.
function renderUnderAToastRegion() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}
