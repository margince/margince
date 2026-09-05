// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { CaptureActivityTab } from "./capture-activity";

// What this surface must never do is state a fact it does not have. Every test
// here is about that: an absent subject is not the same as a subject the
// installation chose not to keep, and a funnel of zeros is not an absent one.

const ROW = {
  id: "01930000-0000-7000-8000-00000000c001",
  connector: "gmail",
  outcome: "internal",
  // The bucket the server counts this row under, and what the screen filters
  // on. Equal to `outcome` for every message whose sender raised no question;
  // the two differ only where a verdict has since settled one, which is what
  // the cases about the fold say for themselves.
  outcome_now: "internal",
  reason: "internal_only",
  activity_id: null,
  resolution: null,
  counterparty: null,
  subject: null,
  occurred_at: "2026-08-15T09:12:00Z",
};

function windowBody(over: Record<string, unknown> = {}) {
  return {
    funnel: { captured: 12, internal: 3, suppressed: 0, deferred: 5, fault: 0 },
    data: [ROW],
    page: { next_cursor: null },
    payload_capture_enabled: false,
    window_hours: 24,
    ...over,
  };
}

// The message a trace's citation opens, in the shape the presentation read
// answers with.
function presentationBody() {
  return {
    id: "01930000-0000-7000-8000-00000000d001",
    lifecycle: "delivered",
    occurred_at: "2026-08-15T09:12:00Z",
    summary: {
      activity_id: "01930000-0000-7000-8000-00000000d001",
      occurred_at: "2026-08-15T09:12:00Z",
      version: 1,
      subject: "Re: the pilot",
      preview: "they wrote",
      display_status: "team",
      move: "none",
      attachment_count: 0,
    },
    body: "they wrote",
    thread_key: null,
    from: [],
    to: [],
    cc: [],
    bcc: [],
    bcc_withheld: false,
    attachments: [],
    links: [],
    thread: { members: [], next_cursor: null },
    access: {
      content_state: "available",
      display_status: "team",
      audience: "workspace",
      can_change: false,
      change_mode: "none",
    },
    can_reply: false,
    can_relink: false,
    version: 1,
  };
}

// The ladder the drawer reads. Two rungs the client knows and ONE it does not,
// because the surface's whole evolution claim is that a stage added by a newer
// server still renders rather than vanishing.
function ladderBody(over: Record<string, unknown> = {}) {
  return {
    activity_id: null,
    connector: "gmail",
    payload_capture_enabled: false,
    retention_hours: 24,
    stages: [
      {
        stage: "internal_drop",
        order: 40,
        subject_kind: "message",
        status: "skipped",
        reason: "internal_only",
      },
      {
        stage: "attention_label",
        order: 100,
        subject_kind: "message",
        status: "skipped",
        reason: "transport_not_read",
      },
      {
        stage: "a_stage_from_the_future",
        order: 130,
        subject_kind: "message",
        status: "done",
        label: "Sentiment scoring",
        reason: "invented_reason",
        reason_text: "the sentiment pass read it",
      },
    ],
    ...over,
  };
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// A per-file harness, per house convention (jobhealth.test.tsx's shape): the
// routes this surface can call, and nothing else.
// `allow` rather than a role name: the fixture does not infer grants from
// roles, deliberately — a screen must gate on the grant the server actually
// sent, not on a role a test asserted.
function renderTab(
  body: Record<string, unknown>,
  allow: GrantSpec = {},
  exclusions: readonly Record<string, unknown>[] = [],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      if (key === "GET /me") return jsonResponse(meFixture({ allow }));
      if (key === "GET /capture/activity") return jsonResponse(body);
      // The block list is composed on this page now, so the harness owes it a
      // route: without one it renders against `{}` and takes the whole card
      // down with it, which is what every test in this file saw first.
      if (key === "GET /capture/exclusions") {
        return jsonResponse({ data: exclusions });
      }
      if (key === "GET /capture/activity/workspace") {
        return jsonResponse(windowBody({ data: [], funnel: {} }));
      }
      if (key === "GET /channel-providers") {
        return jsonResponse({
          data: [
            {
              provider: "gmail",
              label: "Gmail",
              credential_model: "per_member",
              supplies_transport: true,
            },
          ],
        });
      }
      if (key.startsWith("GET /capture/traces/")) {
        return jsonResponse(ladderBody());
      }
      // The message a trace's citation opens. Answered here rather than left
      // to the catch-all below, because `{}` is not an EmailPresentation and
      // the drawer would throw on a shape the server never sends.
      if (key.includes("/email-presentation")) {
        return jsonResponse(presentationBody());
      }
      return jsonResponse({});
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const ui: ReactNode = (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CaptureActivityTab />
      </LocaleProvider>
    </QueryClientProvider>
  );
  return rtlRender(ui);
}

// The per-message log is behind a disclosure, so a test that wants a row has to
// open it the way a reader does. jsdom renders `<details>` children whether or
// not it is open, so reaching straight for a row would assert about markup a
// reader cannot see — the exact confusion this surface exists to avoid.
async function logDisclosure(): Promise<HTMLDetailsElement> {
  // `find`, not `get`: the summary is drawn by the window query, so a helper
  // that read synchronously would race the fetch every caller has just started.
  const summary = await screen.findByText("Messages");
  const details = summary.closest("details");
  if (!(details instanceof HTMLDetailsElement)) {
    throw new Error("the per-message log is not inside a disclosure");
  }
  return details;
}

async function openLog(): Promise<HTMLDetailsElement> {
  const details = await logDisclosure();
  if (!details.open) {
    await userEvent.click(await screen.findByText("Messages"));
  }
  return details;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("capture activity", () => {
  it("counts every outcome, including the ones that are zero", async () => {
    renderTab(windowBody());
    // Zero is a reading: "nothing was dropped as internal today" is exactly
    // what somebody opens this page to confirm, so the slot must be drawn.
    expect(await screen.findByText("12")).toBeInTheDocument();
    expect(screen.getAllByText("0").length).toBeGreaterThan(0);
  });

  it("says once that the installation stores no sender, not once per row", async () => {
    // The old spelling put "content not stored" on every row, which reads as a
    // fact about THAT message — as though this one arrived without a sender.
    // It is the deployment having stored an address for none of them, so it is
    // said once, above the rows, about the installation.
    renderTab(windowBody({ data: [ROW, { ...ROW, id: "row-2" }] }));
    await openLog();
    const note = await screen.findAllByText(
      /does not record who sent a message/i,
    );
    expect(note).toHaveLength(1);
  });

  it("distinguishes an absent payload from a posture that stores none", async () => {
    // Payload capture is ON and this row still carries nothing — an erased
    // subject. That absence IS about this message, so the row says so, and the
    // installation-wide note must not appear to blame the operator's posture
    // for a deletion somebody requested.
    renderTab(windowBody({ payload_capture_enabled: true }));
    await openLog();
    expect(await screen.findByText(/no sender recorded/i)).toBeInTheDocument();
    expect(
      screen.queryByText(/does not record who sent a message/i),
    ).not.toBeInTheDocument();
  });

  it("explains a reason that changes what the outcome means", async () => {
    renderTab(
      windowBody({
        data: [
          {
            ...ROW,
            outcome: "deferred",
            outcome_now: "deferred",
            reason: "deferral_capped",
          },
        ],
      }),
    );
    // "Waiting on a verdict" alone would tell the reader to wait for an answer
    // that is never coming.
    expect(
      await screen.findByText(/no verdict is coming/i),
    ).toBeInTheDocument();
  });

  it("says where the numbers start, so the funnel is not read as everything", async () => {
    renderTab(windowBody());
    expect(
      await screen.findByText(/filtered on its own side/i),
    ).toBeInTheDocument();
  });

  it("hides the shared-channel toggle from a seat without the grant", async () => {
    renderTab(windowBody());
    await screen.findByText("Outcomes");
    expect(screen.queryByText(/shared channels/i)).not.toBeInTheDocument();
  });

  it("offers the shared-channel toggle to a seat that holds capture_trace", async () => {
    renderTab(windowBody(), { capture_trace: ["read"] });
    expect(await screen.findByText(/shared channels/i)).toBeInTheDocument();
  });

  it("renders a suppression reason as a sentence, never as a raw key", async () => {
    // The catalog falls back to the KEY when one is missing, so a missing entry
    // is invisible until somebody sees a row. This one shipped that way.
    renderTab(
      windowBody({
        data: [
          {
            ...ROW,
            outcome: "suppressed",
            outcome_now: "suppressed",
            reason: "transactional_infra",
          },
        ],
      }),
    );
    expect(await screen.findByText(/mail infrastructure/i)).toBeInTheDocument();
    expect(screen.queryByText(/captureActivity\./)).not.toBeInTheDocument();
  });

  it("renders nothing for a reason it does not know", async () => {
    // A row written by a newer binary. Rendering the key would show a member an
    // identifier; the honest answer is that this screen does not know.
    renderTab(
      windowBody({
        data: [
          {
            ...ROW,
            outcome: "captured",
            outcome_now: "captured",
            reason: "teleported",
          },
        ],
      }),
    );
    await openLog();
    expect(screen.queryByText(/teleported/)).not.toBeInTheDocument();
  });

  it("does not say a capped deferral is waiting for a verdict", async () => {
    // The outcome and its own explanation must not argue: nothing is queued and
    // no verdict is coming, so "Waiting on a verdict" above "no verdict is
    // coming" is the screen contradicting itself.
    renderTab(
      windowBody({
        data: [
          {
            ...ROW,
            outcome: "deferred",
            outcome_now: "deferred",
            reason: "deferral_capped",
          },
        ],
      }),
    );
    const row = within(await screen.findByRole("list"));
    expect(row.getByText(/not queued/i)).toBeInTheDocument();
    expect(row.queryByText(/waiting on a verdict/i)).not.toBeInTheDocument();
  });

  it("keeps the deferred bucket's tense, because the count is only the waiting", async () => {
    // The bucket used to be one number over both the settled and the
    // still-waiting, so a tense in its label was a claim it could not support
    // and it read "Sent for a verdict" instead. The server now counts a judged
    // sender's message under the answer, so `deferred` IS the waiting — and the
    // label a person opens this page for is the one it could not say before.
    renderTab(
      windowBody({
        funnel: {
          captured: 1,
          internal: 0,
          suppressed: 0,
          deferred: 1,
          fault: 0,
        },
        data: [
          {
            ...ROW,
            outcome: "deferred",
            // Judged real: the pipeline deferred it, the verdict settled it,
            // and the server counts it under `captured` rather than leaving it
            // in a bucket labelled as waiting.
            outcome_now: "captured",
            reason: null,
            resolution: {
              status: "real",
              kind: "person",
              resolved_at: "2026-08-15T09:13:00Z",
            },
          },
        ],
      }),
    );
    const funnel = within(await screen.findByTestId("capture-activity-funnel"));
    expect(
      funnel.getByText(/waiting on a sender verdict/i),
    ).toBeInTheDocument();
    expect(funnel.queryByText(/sent for a verdict/i)).not.toBeInTheDocument();
  });

  it("says when the sender pass runs while messages are waiting on it", async () => {
    // The complaint this answers: a freshly connected mailbox shows every
    // message waiting, and nothing on the screen says when that changes — so
    // the only readings available are "broken" and "slow", and the pipeline is
    // neither. The cadence comes off the wire, from the job's own declaration,
    // rather than being written down beside the screen where it would drift
    // from the clock the worker actually keeps.
    renderTab(
      windowBody({
        funnel: {
          captured: 0,
          internal: 0,
          suppressed: 0,
          deferred: 3,
          fault: 0,
        },
        sender_verdict: {
          every_seconds: 3600,
          running: false,
          next_pass_at: "2026-08-15T22:21:00Z",
        },
      }),
    );
    const note = await screen.findByTestId("verdict-pass-senders");
    expect(note).toHaveTextContent(/every 60 minutes/i);
    expect(note).toHaveTextContent(/next pass/i);
  });

  it("says a pass is running rather than naming a time it is already past", async () => {
    renderTab(
      windowBody({
        funnel: {
          captured: 0,
          internal: 0,
          suppressed: 0,
          deferred: 3,
          fault: 0,
        },
        sender_verdict: {
          every_seconds: 3600,
          running: true,
          next_pass_at: "2026-08-15T22:21:00Z",
        },
      }),
    );
    const note = await screen.findByTestId("verdict-pass-senders");
    expect(note).toHaveTextContent(/a pass is running now/i);
    expect(note).not.toHaveTextContent(/next pass/i);
  });

  it("says nothing about a pass when nothing is waiting on it", async () => {
    // The note is the exception, not the furniture. A cadence over a window
    // with nothing outstanding is a line a reader learns to skip, and the day
    // it matters they skip it then too.
    renderTab(
      windowBody({
        funnel: {
          captured: 4,
          internal: 0,
          suppressed: 0,
          deferred: 0,
          fault: 0,
        },
        sender_verdict: {
          every_seconds: 3600,
          running: false,
          next_pass_at: "2026-08-15T22:21:00Z",
        },
      }),
    );
    await screen.findByTestId("capture-activity-funnel");
    expect(
      screen.queryByTestId("verdict-pass-senders"),
    ).not.toBeInTheDocument();
  });

  it("says nothing about a pass this deployment cannot time", async () => {
    // A server too old to send the clock, or one that composed no queue to ask.
    // Silence is the honest answer; an invented time is worse than none.
    renderTab(
      windowBody({
        funnel: {
          captured: 0,
          internal: 0,
          suppressed: 0,
          deferred: 3,
          fault: 0,
        },
      }),
    );
    await screen.findByTestId("capture-activity-funnel");
    expect(
      screen.queryByTestId("verdict-pass-senders"),
    ).not.toBeInTheDocument();
  });

  it("filters on the bucket the counters use, not on what was recorded", async () => {
    // The filter label reads "Showing N of M <outcome>", with N from the rows
    // and M from the tiles. Counting the two different ways is what put
    // "Showing 49 of 49 Waiting on a verdict" over forty-nine rows each
    // reading "judged noise".
    const user = userEvent.setup();
    renderTab(
      windowBody({
        funnel: {
          captured: 0,
          internal: 0,
          suppressed: 1,
          deferred: 0,
          fault: 0,
        },
        data: [
          {
            ...ROW,
            outcome: "deferred",
            outcome_now: "suppressed",
            reason: null,
            resolution: {
              status: "noise",
              kind: "newsletter",
              resolved_at: "2026-08-15T09:13:00Z",
            },
          },
        ],
      }),
    );
    await openLog();
    await user.click(
      screen.getByRole("button", { name: /^no person created/i }),
    );
    expect(
      await screen.findByText(/showing 1 of 1 no person created/i),
    ).toBeInTheDocument();
  });

  it("does not say a settled deferral is still waiting for its verdict", async () => {
    // The same contradiction as the capped case, from the other direction: the
    // ledger has answered, the answer is on the row, and the outcome beside it
    // must not still be asking the question.
    renderTab(
      windowBody({
        data: [
          {
            ...ROW,
            outcome: "deferred",
            outcome_now: "deferred",
            reason: null,
            resolution: {
              status: "real",
              kind: "person",
              resolved_at: "2026-08-15T09:13:00Z",
            },
          },
        ],
      }),
    );
    const row = within(await screen.findByRole("list"));
    expect(row.getByText(/sent for a verdict/i)).toBeInTheDocument();
    expect(row.getByText(/judged a real person/i)).toBeInTheDocument();
    expect(row.queryByText(/waiting on a verdict/i)).not.toBeInTheDocument();
  });

  it("still says a deferral with no answer yet is waiting", async () => {
    // The control for the test above: the label is settled-vs-waiting, not
    // "deferrals never say waiting".
    renderTab(
      windowBody({
        data: [
          {
            ...ROW,
            outcome: "deferred",
            outcome_now: "deferred",
            reason: null,
            resolution: { status: "pending", kind: null, resolved_at: null },
          },
        ],
      }),
    );
    const row = within(await screen.findByRole("list"));
    // And it names WHICH verdict. Two run on this pipeline, ten minutes and an
    // hour apart, and "waiting on a verdict" named neither.
    expect(row.getByText(/waiting on a sender verdict/i)).toBeInTheDocument();
  });

  // What a rep meets on this page, in order. The block list is the one control
  // they came for; the counters say what the window did; the per-message log is
  // a diagnostic and sits behind a disclosure so it is available without being
  // the first thing on the page. Closed is the default and is the assertion:
  // `<details>` renders its children either way, so a test that merely FINDS a
  // row proves nothing about whether a reader can see it.
  it("puts the block list and the counters ahead of the per-message log", async () => {
    renderTab(windowBody());
    const counters = (await screen.findByText("Outcomes")).closest(
      ".settingrow",
    );
    expect(counters).not.toBeNull();
    if (counters instanceof HTMLElement) {
      expect(
        within(counters).getByTestId("capture-activity-funnel"),
      ).toBeInTheDocument();
      expect(
        within(counters).getByText(/filtered on its own side/i),
      ).toBeInTheDocument();
    }
    // The block list is on this page at all — it used to live two tabs away
    // under Organization, behind a door most seats cannot open.
    expect(await screen.findByText("Keep out of capture")).toBeInTheDocument();
    // And the log is closed, so nothing about one message is on screen until
    // somebody asks for it.
    const log = await logDisclosure();
    expect(log.open).toBe(false);
    await openLog();
    expect(log.open).toBe(true);
    expect(within(log).getByRole("list")).toBeInTheDocument();
  });

  it("reports an empty window as empty rather than as a failure", async () => {
    renderTab(windowBody({ data: [], funnel: {} }));
    expect(
      await screen.findByText(/no capture activity in the last 24 hours/i),
    ).toBeInTheDocument();
  });
});

describe("the pipeline drill-down", () => {
  it("resolves a transport id to its name", async () => {
    // The contract forbids storing a label — two deploys would disagree about
    // the same transport — so the screen resolves it. Before this it printed
    // `ext:dispact-connector:dispact` at a member.
    renderTab(windowBody());
    expect(await screen.findByText("Gmail")).toBeInTheDocument();
    expect(screen.queryByText("gmail")).not.toBeInTheDocument();
  });

  it("says both numbers when a counter is used as a filter", async () => {
    // The counters count the WINDOW; the filter narrows what is LOADED. A bare
    // count under a counter reading 3 would look like the counter was wrong.
    const user = userEvent.setup();
    renderTab(windowBody());
    await openLog();
    await user.click(
      screen.getByRole("button", { name: /dropped as internal/i }),
    );
    expect(await screen.findByText(/showing 1 of 3/i)).toBeInTheDocument();
  });

  it("filters the rows to the outcome that was clicked", async () => {
    const user = userEvent.setup();
    renderTab(
      windowBody({
        data: [
          ROW,
          {
            ...ROW,
            id: "01930000-0000-7000-8000-00000000c002",
            outcome: "captured",
            outcome_now: "captured",
            reason: null,
          },
        ],
      }),
    );
    await openLog();
    await user.click(screen.getByRole("button", { name: /^captured/i }));
    const list = within(await screen.findByRole("list"));
    expect(list.getByText("Captured")).toBeInTheDocument();
    expect(list.queryByText("Dropped as internal")).not.toBeInTheDocument();
  });

  it("opens one message's whole path from its row", async () => {
    const user = userEvent.setup();
    renderTab(windowBody());
    await openLog();
    await user.click(
      screen.getByRole("button", { name: /every step this message/i }),
    );
    expect(
      await screen.findByText(/how this message was handled/i),
    ).toBeInTheDocument();
    // The rung that motivated the whole surface: a step that DECLINED, saying
    // why, where before there was nothing at all.
    expect(await screen.findByText(/reads email only/i)).toBeInTheDocument();
  });

  it("renders a stage this build has never heard of", async () => {
    // The evolution seam. A newer server adds a step; this client has no
    // catalog entry for it and must still show it, from the server's own words
    // — because a stage that vanished would be exactly the silence this
    // surface exists to end.
    const user = userEvent.setup();
    renderTab(windowBody());
    await openLog();
    await user.click(
      screen.getByRole("button", { name: /every step this message/i }),
    );
    expect(await screen.findByText("Sentiment scoring")).toBeInTheDocument();
    expect(screen.getByText(/the sentiment pass read it/i)).toBeInTheDocument();
    // And never the raw key, which is how a missing entry shipped here once.
    expect(screen.queryByText(/pipeline\./)).not.toBeInTheDocument();
  });

  it("says once that no step stored content, rather than per step", async () => {
    const user = userEvent.setup();
    renderTab(windowBody());
    await openLog();
    await user.click(
      screen.getByRole("button", { name: /every step this message/i }),
    );
    expect(
      await screen.findByText(/turned payload capture off/i),
    ).toBeInTheDocument();
  });

  // A trace is read to reconcile what the pipeline did against a message the
  // reader remembers sending. Naming that message, and opening it, is what
  // turns the ladder from a diagnostic into an answer.
  it("names the traced message and opens it", async () => {
    const user = userEvent.setup();
    renderTab(
      windowBody({
        payload_capture_enabled: true,
        data: [
          {
            ...ROW,
            outcome: "captured",
            outcome_now: "captured",
            reason: null,
            activity_id: "01930000-0000-7000-8000-00000000d001",
            counterparty: "dana@acme.test",
            subject: "Re: the pilot",
          },
        ],
      }),
    );
    await openLog();
    await user.click(
      screen.getByRole("button", { name: /every step this message/i }),
    );

    await user.click(
      await screen.findByRole("button", { name: /Re: the pilot/ }),
    );

    // The trace drawer gives way rather than stacking: two right-anchored
    // sheets at once are two focus traps and two Escape handlers over one
    // another, so the reader moves from the trace to the message.
    await waitFor(() =>
      expect(screen.queryByText("Sentiment scoring")).not.toBeInTheDocument(),
    );
  });

  // `activity_id` is the server's gate — set only where a timeline row exists
  // AND this caller may read it. An entry whose message is out of their scope
  // names itself and offers nothing to press, rather than handing back a
  // control that would prove the row exists.
  it("names a traced message it cannot open, without offering to", async () => {
    const user = userEvent.setup();
    renderTab(
      windowBody({
        payload_capture_enabled: true,
        data: [
          {
            ...ROW,
            outcome: "captured",
            outcome_now: "captured",
            reason: null,
            activity_id: null,
            counterparty: "dana@acme.test",
            subject: "Re: the pilot",
          },
        ],
      }),
    );
    await openLog();
    await user.click(
      screen.getByRole("button", { name: /every step this message/i }),
    );

    // The ladder is open, so the absence below is the gate rather than a
    // drawer that never rendered.
    await screen.findByText("Sentiment scoring");
    expect(
      screen.queryByRole("button", { name: /Re: the pilot/ }),
    ).not.toBeInTheDocument();
  });
});
