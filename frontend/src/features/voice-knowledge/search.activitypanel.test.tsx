// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
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
import { LocaleProvider } from "../../i18n";
import { companyNamesAnswer } from "../../testing/companynames.testkit";
import { SearchScreen } from "./search";

// A meeting, call, note or task found by search opens in a panel over the
// results, as an email hit opens in the email one.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

const meetingHit = {
  type: "activity",
  id: "a2",
  title: "Quarterly review with Demo GmbH",
  snippet: "…what we agreed…",
  score: 0.6,
  trust_tier: "authoritative",
};

const meeting = {
  id: "a2",
  kind: "meeting",
  subject: "Quarterly review with Demo GmbH",
  body: "We **agreed** on the rollout.",
  occurred_at: "2026-09-01T09:00:00Z",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  content_state: "available",
  host_user_id: "u-1",
  colleague_ids: ["u-1", "u-2"],
  links: [
    { entity_type: "company", entity_id: "c-1" },
    { entity_type: "contact", entity_id: "p-1" },
    { entity_type: "deal", entity_id: "d-1" },
  ],
};

const urlOf = (input: RequestInfo | URL) =>
  input instanceof Request ? input.url : String(input);

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function stubServer(activity: Record<string, unknown> = meeting) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = urlOf(input);
    if (url.includes("/activities/a2")) return json(activity);
    if (url.includes("/contacts/p-1"))
      return json({ id: "p-1", full_name: "Dana Buyer" });
    const company = companyNamesAnswer(url, { "c-1": "Demo GmbH" });
    if (company) return company;
    if (url.includes("/deals/d-1"))
      return json({ id: "d-1", name: "Demo rollout" });
    if (url.includes("/attachments"))
      return json({ data: [], page: { next_cursor: null, has_more: false } });
    if (url.includes("/users/names"))
      return json({
        data: [
          { id: "u-1", display_name: "Lena Fischer" },
          { id: "u-2", display_name: "Anna Weber" },
        ],
      });
    return json({
      data: [meetingHit],
      page: { next_cursor: null, has_more: false },
    });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("SearchScreen — a meeting, call, note or task hit", () => {
  it("opens a meeting hit in the activity panel with who was there and where it is filed", async () => {
    const fetchMock = stubServer();
    render(<SearchScreen q="quarterly review" />);

    const row = await screen.findByRole("button", {
      name: "Quarterly review with Demo GmbH",
    });
    expect(row.getAttribute("aria-haspopup")).toBe("dialog");
    await userEvent.setup().click(row);

    const panel = await screen.findByRole("dialog");
    expect(
      within(panel).getByRole("heading", {
        name: "Quarterly review with Demo GmbH",
      }),
    ).toBeTruthy();
    expect(within(panel).getByText("Meeting")).toBeTruthy();
    // The body reads as the timeline reads a rep's words: markdown.
    expect(within(panel).getByText("agreed").tagName).toBe("STRONG");
    // A linked contact attended; the company and the deal are where it is filed.
    await within(panel).findByText("Dana Buyer");
    await within(panel).findByText("Demo GmbH");
    await within(panel).findByText("Demo rollout");
    // The host and the colleague the read names, the host once.
    expect(await within(panel).findAllByText("Lena Fischer")).toHaveLength(1);
    await within(panel).findByText("Anna Weber");
    expect(
      fetchMock.mock.calls.some((call) =>
        urlOf(call[0]).includes("email-presentation"),
      ),
    ).toBe(false);
  });

  // The palette's half: the address names the activity and the panel to open.
  it("opens the activity the address names in the activity panel, on arrival", async () => {
    const fetchMock = stubServer();
    render(
      <SearchScreen
        q="quarterly review"
        openActivityId="a2"
        openAs="activity"
      />,
    );

    const panel = await screen.findByRole("dialog");
    await within(panel).findByRole("heading", {
      name: "Quarterly review with Demo GmbH",
    });
    expect(
      fetchMock.mock.calls.some((call) =>
        urlOf(call[0]).includes("email-presentation"),
      ),
    ).toBe(false);
  });

  // A reader outside the audience learns there is an activity, and nothing of
  // its words or who took part.
  it("shows a withheld activity as withheld, with no body and no colleagues", async () => {
    stubServer({
      ...meeting,
      content_state: "withheld",
      subject: null,
      body: null,
      colleague_ids: undefined,
      host_user_id: null,
    });
    render(
      <SearchScreen
        q="quarterly review"
        openActivityId="a2"
        openAs="activity"
      />,
    );

    const panel = await screen.findByRole("dialog");
    await within(panel).findByText("Activity content is withheld");
    expect(within(panel).queryByText("agreed")).toBeNull();
    expect(within(panel).queryByText("Colleagues")).toBeNull();
  });
});

// Every activity kind but a message opens the same panel, and the kind it
// names comes from the read.
describe("SearchScreen — the activity panel names the kind", () => {
  it.each([
    ["call", "Call"],
    ["note", "Note"],
    ["task", "Task"],
  ])("names a %s as one", async (kind, label) => {
    stubServer({ ...meeting, kind });
    render(
      <SearchScreen
        q="quarterly review"
        openActivityId="a2"
        openAs="activity"
      />,
    );
    const panel = await screen.findByRole("dialog");
    await waitFor(() => expect(within(panel).getByText(label)).toBeTruthy());
  });
});
