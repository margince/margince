// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import type { EntityKind } from "../lib/entity";
import { forgetSeat } from "../lib/storage";
import {
  LatestInboundLine,
  LatestInboundPanel,
  useLatestInbound,
} from "./latestinbound";

type Activity = components["schemas"]["Activity"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  forgetSeat();
});

const AT = "2026-10-27T09:00:00Z";

function activity(id: string, fields: Partial<Activity>): Activity {
  return {
    id,
    kind: "email",
    occurred_at: AT,
    occurred_precision: "instant",
    source: "email",
    captured_by: "connector:email",
    created_at: AT,
    updated_at: AT,
    offers_email: true,
    is_done: false,
    ...fields,
  };
}

const MAIL = activity("mail", {
  direction: "inbound",
  subject: "Re: Pricing for 40 seats",
  email_summary: {
    activity_id: "mail",
    occurred_at: AT,
    occurred_precision: "instant",
    display_status: "workspace",
    attachment_count: 0,
    move: "needs_reply",
    version: 1,
    subject: "Re: Pricing for 40 seats",
    preview: "Can you send the order form today?",
    counterparty: "Jonas Weber",
  },
});

function withProviders(node: React.ReactNode) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the Just in panel", () => {
  it("draws nothing when nothing is waiting", () => {
    const { container } = withProviders(
      <LatestInboundPanel latest={undefined} />,
    );
    expect(container.textContent).toBe("");
  });

  it("names what came in, who sent it and their words, with the row's verbs", () => {
    withProviders(
      <LatestInboundPanel
        latest={{ activity: MAIL, kind: "email" }}
        actions={(row) => <button type="button">Reply to {row.id}</button>}
      />,
    );
    expect(screen.getByText("New email")).toBeTruthy();
    expect(screen.getByText("Jonas Weber")).toBeTruthy();
    expect(screen.getByText("Re: Pricing for 40 seats")).toBeTruthy();
    expect(
      screen.getByText(/Can you send the order form today\?/),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Reply to mail" })).toBeTruthy();
  });

  it("shows a withheld message as what it is, without its words", () => {
    withProviders(
      <LatestInboundPanel
        latest={{
          activity: { ...MAIL, content_state: "withheld" },
          kind: "email",
        }}
      />,
    );
    expect(screen.getByText("New email")).toBeTruthy();
    expect(screen.queryByText("Re: Pricing for 40 seats")).toBeNull();
  });

  it("reads a website form's answers as one line", () => {
    const form = activity("form", {
      kind: "note",
      source: "web_form",
      subject: "Website form: Request a demo",
      body: "We run 45 field reps.\n\nTeam size: 40-60",
    });
    withProviders(
      <LatestInboundPanel latest={{ activity: form, kind: "web_form" }} />,
    );
    expect(screen.getByText("New website form")).toBeTruthy();
    expect(
      screen.getByText("We run 45 field reps. · Team size: 40-60"),
    ).toBeTruthy();
  });

  it("reads a quoted website form as the words the visitor sent", () => {
    const form = activity("form", {
      kind: "note",
      source: "web_form",
      subject: "Website form: Contact form",
      body: "> We would like a demo next week\n>\n> Name: Pat Quill\n> Email: pat@example.test",
    });
    withProviders(
      <LatestInboundPanel latest={{ activity: form, kind: "web_form" }} />,
    );
    expect(
      screen.getByText(
        "We would like a demo next week · Name: Pat Quill · Email: pat@example.test",
      ),
    ).toBeTruthy();
  });
});

describe("the Just in line in the record's head", () => {
  it("takes the reader back to the Overview", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    withProviders(
      <LatestInboundLine
        latest={{ activity: MAIL, kind: "email" }}
        onOpen={onOpen}
      />,
    );
    await user.click(
      screen.getByRole("button", {
        name: "New email: open in Overview",
      }),
    );
    expect(onOpen).toHaveBeenCalledOnce();
  });
});

function Probe({ kind, id }: Readonly<{ kind: EntityKind; id: string }>) {
  const latest = useLatestInbound(kind, id);
  return (
    <>
      <span>{latest ? `${latest.kind}:${latest.activity.id}` : "none"}</span>
      <LatestInboundPanel latest={latest} />
    </>
  );
}

function serveActivities(waitingRows: Activity[], recentRows: Activity[]) {
  const asked: URLSearchParams[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test.local",
      );
      asked.push(url.searchParams);
      const waiting = url.searchParams.get("waiting_reply") === "true";
      return new Response(
        JSON.stringify({
          data: waiting ? waitingRows : recentRows,
          page: { next_cursor: null, has_more: false },
        }),
        { headers: { "Content-Type": "application/json" } },
      );
    }),
  );
  return asked;
}

describe("reading the latest inbound item", () => {
  // Only Date is faked: the reads and clicks still run on real timers.
  beforeEach(() => {
    vi.useFakeTimers({
      toFake: ["Date"],
      now: new Date("2026-10-28T09:00:00Z"),
    });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("asks for the record's waiting reply and its last week, and picks the newer", async () => {
    const laterForm = activity("form", {
      kind: "note",
      source: "web_form",
      occurred_at: "2026-10-27T10:00:00Z",
    });
    const asked = serveActivities([MAIL], [laterForm, MAIL]);
    withProviders(<Probe kind="lead" id="lead-1" />);
    expect(await screen.findByText("web_form:form")).toBeTruthy();
    expect(asked).toHaveLength(2);
    for (const params of asked) {
      expect(params.get("entity_type")).toBe("lead");
      expect(params.get("entity_id")).toBe("lead-1");
      expect(params.get("occurred_after")).toBeTruthy();
    }
  });

  it("sets the item aside once Noticed is pressed, and keeps it aside", async () => {
    const user = userEvent.setup();
    serveActivities([MAIL], [MAIL]);
    withProviders(<Probe kind="contact" id="contact-noticed" />);
    expect(await screen.findByText("email:mail")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Noticed" }));
    expect(screen.getByText("none")).toBeTruthy();
    cleanup();
    withProviders(<Probe kind="contact" id="contact-noticed" />);
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(4));
    expect(screen.getByText("none")).toBeTruthy();
  });

  it("passes over an item once it is older than the window", async () => {
    vi.setSystemTime(new Date("2026-11-04T10:00:00Z"));
    const fresh = activity("fresh", {
      kind: "message",
      direction: "inbound",
      occurred_at: "2026-11-03T10:00:00Z",
    });
    serveActivities([MAIL, fresh], [fresh, MAIL]);
    withProviders(<Probe kind="contact" id="contact-aged" />);
    expect(await screen.findByText("message:fresh")).toBeTruthy();
  });
});
