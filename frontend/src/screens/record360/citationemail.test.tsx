/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { Citations, type Cited } from "./citations";

// A citation that names a message opens THAT message, and a citation that
// cannot open anything is prose.
//
// The defect these hold: the shared renderer decided openability from the
// citation's KIND alone, and `activity` was never in the openable set — so an
// account page that already mounted an email drawer, and a hook that already
// knew how to route an activity to it, drew a chip that reached neither. The
// demo's translation suggestion is the visible case: pressing its source email
// opened a quote popover.

type EmailSummary = components["schemas"]["EmailSummary"];

function summary(overrides: Partial<EmailSummary> = {}): EmailSummary {
  return {
    activity_id: "act-1",
    subject: "Translation fallback decision",
    preview: "Could you confirm which locale wins when the string is missing?",
    occurred_at: "2026-09-03T09:15:00Z",
    direction: "inbound",
    counterparty: "Frédéric de Gombert",
    attachment_count: 0,
    move: "needs_reply",
    display_status: "team",
    version: 1,
    ...overrides,
  };
}

function citedEmail(overrides: Partial<EmailSummary> = {}): Cited {
  const email = summary(overrides);
  return {
    entity_type: "activity",
    entity_id: email.activity_id,
    email_summary: email,
  };
}

function renderCitations(
  evidence: Cited[],
  handlers: {
    onOpenEmail?: (activityId: string) => void;
    onOpenRecord?: (entityType: string, entityId: string) => void;
  } = {},
) {
  render(
    <LocaleProvider initial="en">
      <Citations evidence={evidence} {...handlers} />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("a cited message", () => {
  it("opens the exact message it names", async () => {
    const onOpenEmail = vi.fn();
    renderCitations([citedEmail()], { onOpenEmail });

    await userEvent.click(
      screen.getByRole("button", { name: /Translation fallback decision/ }),
    );

    // The id the drawer is opened with is the assertion. A test asserting only
    // that something was clicked would pass over a renderer that opened the
    // wrong message.
    expect(onOpenEmail).toHaveBeenCalledWith("act-1");
  });

  it("opens on the keyboard too", async () => {
    const onOpenEmail = vi.fn();
    renderCitations([citedEmail()], { onOpenEmail });

    await userEvent.tab();
    await userEvent.keyboard("{Enter}");

    expect(onOpenEmail).toHaveBeenCalledWith("act-1");
  });

  it("names the message rather than its record kind", () => {
    renderCitations([citedEmail()], { onOpenEmail: vi.fn() });

    expect(screen.getByText("Translation fallback decision")).toBeTruthy();
    // The bare kind word is what the citation used to render for every
    // activity on the page — the run of identical labels a subject replaces.
    expect(screen.queryByText("activity")).toBeNull();
  });
});

describe("a message this reader may not read", () => {
  it("says a message is there and offers nothing to press", () => {
    const onOpenEmail = vi.fn();
    renderCitations(
      [
        citedEmail({
          display_status: "withheld",
          subject: null,
          preview: null,
        }),
      ],
      { onOpenEmail },
    );

    // The row stays: drawing nothing would say the exchange never happened.
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByText("Translation fallback decision")).toBeNull();
  });
});

describe("a host that mounts no email drawer", () => {
  it("renders the message as prose rather than a dead control", () => {
    renderCitations([citedEmail()]);

    expect(screen.queryByRole("button")).toBeNull();
    // Still named, so the reader knows which message the claim rests on.
    expect(screen.getByText("Translation fallback decision")).toBeTruthy();
  });
});

describe("an activity the server sent no summary for", () => {
  it("renders exactly as it did before, never as a button", () => {
    // The old-server case, and the not-an-email case, and the
    // reader-has-no-activity-grant case. The server folds all three into an
    // absent summary, and the client may not guess which.
    const onOpenEmail = vi.fn();
    renderCitations(
      [{ entity_type: "activity", entity_id: "act-9", name: "A phone call" }],
      { onOpenEmail },
    );

    expect(screen.getByText("A phone call")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
    expect(onOpenEmail).not.toHaveBeenCalled();
  });
});

describe("evidence mixing a readable message with an unresolved one", () => {
  it("opens the one it can and leaves the other as prose", async () => {
    // The grouping case. Two citations of the same kind used to collapse into
    // one counted chip — "2 activities" — which would hide the openable one
    // behind a label speaking for both.
    const onOpenEmail = vi.fn();
    renderCitations(
      [
        citedEmail(),
        { entity_type: "activity", entity_id: "act-2", name: "A call" },
      ],
      { onOpenEmail },
    );

    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(1);
    await userEvent.click(buttons[0]);
    expect(onOpenEmail).toHaveBeenCalledWith("act-1");
    expect(screen.getByText("A call")).toBeTruthy();
  });
});

describe("a citation that is not a message", () => {
  it("still routes a deal to its own screen", async () => {
    const onOpenRecord = vi.fn();
    renderCitations(
      [{ entity_type: "deal", entity_id: "deal-1", name: "Translation pilot" }],
      { onOpenRecord, onOpenEmail: vi.fn() },
    );

    await userEvent.click(
      screen.getByRole("button", { name: /Translation pilot/ }),
    );

    expect(onOpenRecord).toHaveBeenCalledWith("deal", "deal-1", []);
  });

  it("leaves a company flat, as the page the reader is already on", () => {
    renderCitations(
      [{ entity_type: "company", entity_id: "company-1", name: "Akeneo" }],
      { onOpenRecord: vi.fn(), onOpenEmail: vi.fn() },
    );

    expect(screen.getByText("Akeneo")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });
});

describe("one message cited twice", () => {
  it("is drawn once, like every other repeated citation", () => {
    // The chips have deduplicated by record since they were written; the
    // message path was added beside them and did not. A brief resting four
    // sentences on one thread drew the thread four times — and, because the
    // key is the activity id, drew it four times under one React key.
    renderCitations([citedEmail(), citedEmail()], { onOpenEmail: vi.fn() });

    expect(screen.getAllByRole("button")).toHaveLength(1);
  });

  it("still draws two DIFFERENT messages", () => {
    // The guard on the guard: deduplicating on the wrong key would collapse
    // these two as well, and the count above would not notice.
    renderCitations(
      [
        citedEmail(),
        citedEmail({ activity_id: "act-2", subject: "Second thread" }),
      ],
      { onOpenEmail: vi.fn() },
    );

    expect(screen.getAllByRole("button")).toHaveLength(2);
  });
});
