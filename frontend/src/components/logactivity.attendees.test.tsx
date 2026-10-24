/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { LogActivity } from "./logactivity";
import { MAX_MEETING_NAMES, MeetingAttendeeFields } from "./meetingattendees";

// Everyone in a logged meeting reaches the wire. Each contact is a link, so the
// meeting shows in their History and their company's. Each colleague is an
// attendee, so it counts for their relationship too.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const ME = meFixture({ allow: { activity: ["create"], contact: ["read"] } });

const contacts = {
  data: [
    {
      id: "c-b",
      full_name: "Bea Brandt",
      employer: { company_id: "co-b", company_name: "Brandt AG" },
    },
    {
      id: "c-c",
      full_name: "Cem Celik",
      employer: { company_id: "co-c", company_name: "Celik GmbH" },
    },
    { id: "c-a", full_name: "Ada Open" },
  ],
  page: { next_cursor: null },
};

const users = {
  data: [
    { id: ME.user.id, display_name: "Me Myself", is_agent: false },
    { id: "u-ben", display_name: "Ben Colleague", is_agent: false },
    { id: "agent-1", display_name: "Runner Bot", is_agent: true },
  ],
  page: { next_cursor: null },
};

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
  });
}

// Answers by path, and records the body of the one POST the form sends.
function stubApi(posted: unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://t.local",
      );
      const path = url.pathname.replace(/^\/v1/, "");
      const method = request?.method ?? init?.method ?? "GET";
      if (method === "POST" && path === "/activities") {
        const body = request
          ? await request.json()
          : JSON.parse(String(init?.body));
        posted.push(body);
        return json({ ...body, id: "a-new", captured_by: "human:x" });
      }
      if (path === "/me") return json(ME);
      if (path === "/contacts") return json(contacts);
      if (path === "/users") return json(users);
      return json({ data: [], page: { next_cursor: null } });
    }),
  );
}

function render(entityType: "contact" | "company" | "deal", entityId: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <LogActivity entityType={entityType} entityId={entityId} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function addAttendee(user: UserEvent, label: string, name: RegExp) {
  await user.type(screen.getByLabelText(label), "e");
  await user.click(await screen.findByRole("button", { name }));
}

async function logMeeting(user: UserEvent, posted: unknown[]) {
  await user.type(screen.getByLabelText("Subject *"), "Joint call");
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() => expect(posted).toHaveLength(1));
  return posted[0];
}

describe("everyone on a logged meeting", () => {
  it("links the open contact and every attendee across companies", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    stubApi(posted);
    const invalidate = vi.spyOn(QueryClient.prototype, "invalidateQueries");
    render("contact", "c-a");
    await pickOption(user, screen.getByLabelText("Type"), "Meeting");

    // The open contact is linked already, so it is not offered again.
    await user.type(screen.getByLabelText("Other attendees"), "e");
    await screen.findByRole("button", { name: /Bea Brandt · Brandt AG/ });
    expect(screen.queryByRole("button", { name: /Ada Open/ })).toBeNull();
    await user.click(screen.getByRole("button", { name: /Bea Brandt/ }));
    await addAttendee(user, "Other attendees", /Cem Celik/);

    expect(await logMeeting(user, posted)).toMatchObject({
      kind: "meeting",
      links: [
        { entity_type: "contact", entity_id: "c-a" },
        { entity_type: "contact", entity_id: "c-b" },
        { entity_type: "contact", entity_id: "c-c" },
      ],
    });
    // Their companies' History reads too: the meeting reaches them through
    // each attendee's employer, which the form does not know.
    await waitFor(() =>
      expect(invalidate).toHaveBeenCalledWith({ queryKey: ["company360"] }),
    );
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["activities", "company"],
    });
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["activities", "contact", "c-b"],
    });
  });

  it("keeps a deal's link beside the contacts who attended", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    stubApi(posted);
    render("deal", "d-1");
    await pickOption(user, screen.getByLabelText("Type"), "Call");
    await addAttendee(user, "Other attendees", /Bea Brandt/);

    expect(await logMeeting(user, posted)).toMatchObject({
      links: [
        { entity_type: "deal", entity_id: "d-1" },
        { entity_type: "contact", entity_id: "c-b" },
      ],
    });
  });

  it("sends several attendees from a company page, and no company link", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    stubApi(posted);
    render("company", "co-b");
    await pickOption(user, screen.getByLabelText("Type"), "Meeting");
    await addAttendee(user, "Attendees", /Bea Brandt/);
    await addAttendee(user, "Attendees", /Cem Celik/);

    expect(await logMeeting(user, posted)).toMatchObject({
      links: [
        { entity_type: "contact", entity_id: "c-b" },
        { entity_type: "contact", entity_id: "c-c" },
      ],
    });
  });

  it("drops an attendee the reader removed", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    stubApi(posted);
    render("company", "co-b");
    await pickOption(user, screen.getByLabelText("Type"), "Meeting");
    await addAttendee(user, "Attendees", /Bea Brandt/);
    await addAttendee(user, "Attendees", /Cem Celik/);
    await user.click(screen.getByRole("button", { name: /Remove Bea Brandt/ }));

    expect(await logMeeting(user, posted)).toMatchObject({
      links: [{ entity_type: "contact", entity_id: "c-c" }],
    });
  });

  it("names the colleagues who attended, never the logger or an agent seat", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    stubApi(posted);
    render("contact", "c-a");
    await pickOption(user, screen.getByLabelText("Type"), "Meeting");
    const colleagues = screen.getByLabelText("Colleagues who attended");
    await user.click(colleagues);
    expect(screen.queryByRole("option", { name: "Me Myself" })).toBeNull();
    expect(screen.queryByRole("option", { name: "Runner Bot" })).toBeNull();
    await user.keyboard("{Escape}");
    await pickOption(user, colleagues, "Ben Colleague");

    expect(await logMeeting(user, posted)).toMatchObject({
      attendee_user_ids: ["u-ben"],
    });
  });

  it("sends no colleagues on a note", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    stubApi(posted);
    render("contact", "c-a");
    await pickOption(user, screen.getByLabelText("Type"), "Meeting");
    await pickOption(
      user,
      screen.getByLabelText("Colleagues who attended"),
      "Ben Colleague",
    );
    await pickOption(user, screen.getByLabelText("Type"), "Note");

    const body = await logMeeting(user, posted);
    expect(body).not.toHaveProperty("attendee_user_ids");
    expect(body).toMatchObject({
      links: [{ entity_type: "contact", entity_id: "c-a" }],
    });
  });
});

// The server refuses a 26th link or colleague, so the form stops offering one
// before Save rather than after it.
describe("a full meeting", () => {
  it("offers no further contact or colleague at the server's cap", async () => {
    stubApi([]);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const ids = (count: number, prefix: string) =>
      Array.from({ length: count }, (_, i) => `${prefix}-${i}`);
    const fields = (room: number) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <MeetingAttendeeFields
            entityType="contact"
            entityId="c-a"
            value={{
              contacts: ids(room - 1, "c").map((id) => ({ id, name: id })),
              colleagueIds: ids(room, "u"),
            }}
            onChange={() => {}}
          />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const contactsBox = () => screen.getByLabelText("Other attendees");
    const colleaguesBox = () =>
      screen.getByLabelText("Colleagues who attended");

    // One short of the cap, and the roster loaded: both still offer more.
    const view = rtlRender(fields(MAX_MEETING_NAMES - 1));
    await waitFor(() =>
      expect(colleaguesBox().hasAttribute("disabled")).toBe(false),
    );
    expect(contactsBox().hasAttribute("disabled")).toBe(false);

    view.rerender(fields(MAX_MEETING_NAMES));
    expect(contactsBox().hasAttribute("disabled")).toBe(true);
    expect(colleaguesBox().hasAttribute("disabled")).toBe(true);
  });
});
