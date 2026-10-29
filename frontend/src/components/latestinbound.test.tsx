// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import type { EntityKind } from "../lib/entity";
import { OpenSourceProvider } from "../lib/opensource";
import { forgetSeat } from "../lib/storage";
import {
  type JustIn,
  JustInScope,
  LatestInboundLine,
  RecordNextBar,
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

const MAIL_IN: JustIn = {
  activity: MAIL,
  kind: "email",
  accepts_request: true,
};

function withProviders(node: ReactNode, openSource = vi.fn()) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <OpenSourceProvider openSource={openSource}>{node}</OpenSourceProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function scoped(latest: JustIn | undefined, bar: ReactNode) {
  return (
    <JustInScope
      latest={latest}
      entityType="contact"
      entityId="contact-1"
      actions={(row) => <button type="button">Reply to {row.id}</button>}
    >
      {bar}
    </JustInScope>
  );
}

const TASK_BAR = {
  title: "Send the pilot results",
  sub: "Due 30 Oct",
} as const;

describe("a record's Next bar with what just came in", () => {
  it("is the plain bar when nothing came in", () => {
    withProviders(scoped(undefined, <RecordNextBar />));
    expect(screen.getByText("Nothing is due next.")).toBeTruthy();
  });

  it("is the plain bar outside a record page", () => {
    withProviders(<RecordNextBar {...TASK_BAR} />);
    expect(screen.getByText("Send the pilot results")).toBeTruthy();
    expect(screen.queryByText("New email")).toBeNull();
  });

  it("takes the bar for the item when nothing urgent is next", () => {
    withProviders(
      scoped({ ...MAIL_IN, notice: vi.fn() }, <RecordNextBar {...TASK_BAR} />),
    );
    expect(screen.queryByText("Nothing is due next.")).toBeNull();
    expect(screen.queryByText("Send the pilot results")).toBeNull();
    expect(screen.getByText("New email")).toBeTruthy();
    expect(screen.getByText("Re: Pricing for 40 seats")).toBeTruthy();
    expect(screen.getByText("Can you send the order form today?")).toBeTruthy();
    expect(screen.getByText(/Jonas Weber/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Reply to mail" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Make it a task" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Noticed" })).toBeTruthy();
  });

  it("stays one line above a bar that is late", () => {
    withProviders(
      scoped(
        MAIL_IN,
        <RecordNextBar
          {...TASK_BAR}
          late={{ label: "3 days late", tone: "danger" }}
        />,
      ),
    );
    expect(screen.getByText("Send the pilot results")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Read email" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Make it a task" })).toBeNull();
  });

  it("stays one line above a bar its page calls urgent", () => {
    withProviders(scoped(MAIL_IN, <RecordNextBar {...TASK_BAR} urgent />));
    expect(screen.getByText("Send the pilot results")).toBeTruthy();
    expect(screen.getByText("Re: Pricing for 40 seats")).toBeTruthy();
  });

  it("leaves a bar alone whose move already cites the item", () => {
    withProviders(
      scoped(MAIL_IN, <RecordNextBar {...TASK_BAR} urgent about={["mail"]} />),
    );
    expect(screen.getByText("Send the pilot results")).toBeTruthy();
    expect(screen.queryByText("Re: Pricing for 40 seats")).toBeNull();
  });

  it("opens the item from the bar and from the line", async () => {
    const user = userEvent.setup();
    const openSource = vi.fn();
    withProviders(scoped(MAIL_IN, <RecordNextBar />), openSource);
    await user.click(screen.getByRole("button", { name: "Read email" }));
    expect(openSource).toHaveBeenCalledWith("mail");
    cleanup();
    withProviders(scoped(MAIL_IN, <RecordNextBar urgent />), openSource);
    await user.click(screen.getByRole("button", { name: "Read email" }));
    expect(openSource).toHaveBeenCalledTimes(2);
  });

  it("shows a withheld message as what it is, with nothing to open or take on", () => {
    withProviders(
      scoped(
        { ...MAIL_IN, activity: { ...MAIL, content_state: "withheld" } },
        <RecordNextBar />,
      ),
    );
    expect(screen.getAllByText("New email").length).toBeGreaterThan(0);
    expect(screen.queryByText("Re: Pricing for 40 seats")).toBeNull();
    expect(screen.queryByRole("button", { name: "Read email" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Make it a task" })).toBeNull();
  });

  it("reads a website form's answers as one line", () => {
    const form = activity("form", {
      kind: "note",
      source: "web_form",
      subject: "Website form: Request a demo",
      body: "We run 45 field reps.\n\nTeam size: 40-60",
    });
    withProviders(
      scoped(
        { activity: form, kind: "web_form", accepts_request: false },
        <RecordNextBar />,
      ),
    );
    expect(screen.getByText("New website form")).toBeTruthy();
    expect(
      screen.getByText("We run 45 field reps. · Team size: 40-60"),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Read form" })).toBeTruthy();
  });

  it("keeps a chat message's own characters, which are not markdown", () => {
    const chat = activity("chat", {
      kind: "message",
      body: "> 10 units\nby Friday",
    });
    withProviders(
      scoped(
        { activity: chat, kind: "message", accepts_request: false },
        <RecordNextBar />,
      ),
    );
    expect(screen.getByText("> 10 units · by Friday")).toBeTruthy();
  });
});

function serveTasks() {
  const posted: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : undefined;
      const body = request ? await request.text() : String(init?.body ?? "");
      posted.push(JSON.parse(body));
      return new Response(JSON.stringify(activity("task", { kind: "task" })), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return posted;
}

describe("making the item a task", () => {
  it("accepts the request, so completing the task settles it", async () => {
    const user = userEvent.setup();
    const posted = serveTasks();
    withProviders(scoped(MAIL_IN, <RecordNextBar />));
    await user.click(screen.getByRole("button", { name: "Make it a task" }));
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toEqual({
      subject: "Reply: Re: Pricing for 40 seats",
      source: "manual",
      request_activity_id: "mail",
    });
  });

  it("makes a task from anything that is no request", async () => {
    const user = userEvent.setup();
    const posted = serveTasks();
    const form = activity("form", {
      kind: "note",
      source: "web_form",
      subject: "Website form: Request a demo",
    });
    withProviders(
      scoped(
        { activity: form, kind: "web_form", accepts_request: false },
        <RecordNextBar />,
      ),
    );
    await user.click(screen.getByRole("button", { name: "Make it a task" }));
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toEqual({
      subject: "Follow up: Website form: Request a demo",
      source: "manual",
      source_activity_id: "form",
    });
  });
});

describe("the Just in line in the record's head", () => {
  it("takes the reader back to the Overview", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    withProviders(<LatestInboundLine latest={MAIL_IN} onOpen={onOpen} />);
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
      {latest?.notice && (
        <button type="button" onClick={latest.notice}>
          Noticed
        </button>
      )}
    </>
  );
}

function serveLatest(items: JustIn[]) {
  const asked: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      asked.push(
        new URL(
          input instanceof Request ? input.url : String(input),
          "https://test.local",
        ),
      );
      return new Response(JSON.stringify({ data: items }), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return asked;
}

describe("reading the latest inbound item", () => {
  const chat: JustIn = {
    activity: activity("chat", { kind: "message", direction: "inbound" }),
    kind: "message",
    accepts_request: false,
  };

  it("asks the server once for the record, and leads with its newest item", async () => {
    const asked = serveLatest([MAIL_IN, chat]);
    withProviders(<Probe kind="lead" id="lead-1" />);
    expect(await screen.findByText("email:mail")).toBeTruthy();
    expect(asked).toHaveLength(1);
    expect(asked[0].pathname).toMatch(/\/activities\/latest-inbound$/);
    expect(asked[0].searchParams.get("entity_type")).toBe("lead");
    expect(asked[0].searchParams.get("entity_id")).toBe("lead-1");
  });

  it("sets the item aside once Noticed is pressed, and keeps it aside", async () => {
    const user = userEvent.setup();
    serveLatest([MAIL_IN, chat]);
    withProviders(<Probe kind="contact" id="contact-noticed" />);
    expect(await screen.findByText("email:mail")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Noticed" }));
    expect(screen.getByText("message:chat")).toBeTruthy();
    cleanup();
    withProviders(<Probe kind="contact" id="contact-noticed" />);
    expect(await screen.findByText("message:chat")).toBeTruthy();
  });
});
