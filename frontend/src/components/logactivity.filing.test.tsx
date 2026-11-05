/** @vitest-environment happy-dom */
import { QueryClient } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { LogActivity } from "./logactivity";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

type FilingOptions = components["schemas"]["ActivityDealFilingOptions"];

function mount(options: FilingOptions, returnedDeal?: string) {
  const posted: unknown[] = [];
  const urls: URL[] = [];
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["deal", "d1"], { id: "d1", last_activity_at: null });
  client.setQueryData(["deal", "d2"], { id: "d2", last_activity_at: null });
  installFetchStub({
    "GET /me": meRoute({
      activity: ["read", "create"],
      contact: ["read"],
      company: ["read"],
      deal: ["read"],
    }),
    "GET /activities/deal-filing": () => jsonResponse(options),
    "GET /deals/d1": () => jsonResponse({ id: "d1", name: "Only open deal" }),
    "GET /deals": () =>
      jsonResponse({
        data: [{ id: "d2", name: "Chosen renewal" }],
        page: { has_more: false },
      }),
    "POST /activities": (body) => {
      posted.push(body);
      return jsonResponse(
        {
          id: "logged-1",
          kind: "call",
          is_done: false,
          offers_email: false,
          occurred_at: "2026-01-10T09:00:00Z",
          created_at: "2026-01-10T09:00:00Z",
          updated_at: "2026-01-10T09:00:00Z",
          source: "manual",
          captured_by: "human:u1",
          links: returnedDeal
            ? [{ entity_type: "deal", entity_id: returnedDeal }]
            : [],
        },
        201,
      );
    },
  });
  const routed = globalThis.fetch;
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    urls.push(
      new URL(
        input instanceof Request ? input.url : String(input),
        "https://test.local",
      ),
    );
    return routed(input, init);
  });
  render(
    <StoryProviders client={client}>
      <LogActivity entityType="contact" entityId="p1" askedKind="call" />
    </StoryProviders>,
  );
  return { posted, urls, client };
}

async function save(user: ReturnType<typeof userEvent.setup>) {
  await waitFor(() =>
    expect(screen.getByRole("button", { name: en["log.save"] })).toHaveProperty(
      "disabled",
      false,
    ),
  );
  await user.click(screen.getByRole("button", { name: en["log.save"] }));
}

describe("filing a logged call from its contact", () => {
  it("lists this company's open deals on focus before typing", async () => {
    const { urls } = mount({ choice_required: true, company_id: "o1" });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("searchbox", { name: en["log.dealFiling"] }),
    );
    expect(
      await screen.findByRole("button", { name: "Chosen renewal" }),
    ).toBeTruthy();
    const request = urls.find(
      (url) =>
        url.pathname.endsWith("/deals") &&
        url.searchParams.get("company_id") === "o1",
    );
    expect(request?.searchParams.get("status")).toBe("open");
    expect(request?.searchParams.get("q")).toBe("");
  });

  it("requests the server's one-deal rule and refreshes the actual deal returned by the write", async () => {
    const { posted, client } = mount(
      { choice_required: false, company_id: "o1", suggested_deal_id: "d1" },
      "d2",
    );
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText(en["log.subject"]),
      "Renewal call",
    );
    expect(await screen.findByText("Only open deal")).toBeTruthy();
    await save(user);
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      deal_filing: {
        record_type: "contact",
        record_id: "p1",
        skip: false,
        deal_id: "d1",
      },
      links: [{ entity_type: "contact", entity_id: "p1" }],
    });
    await waitFor(() =>
      expect(client.getQueryState(["deal", "d2"])?.isInvalidated).toBe(true),
    );
  });

  it("requires an explicit choice when several deals are open and sends none when requested", async () => {
    const options: FilingOptions = { choice_required: true, company_id: "o1" };
    const { posted, client } = mount(options);
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText(en["log.subject"]),
      "General call",
    );
    await screen.findAllByText(en["log.dealChoice"]);
    expect(screen.getByRole("button", { name: en["log.save"] })).toHaveProperty(
      "disabled",
      true,
    );
    const form = screen.getByLabelText(en["log.subject"]).closest("form");
    if (form === null) throw new Error("The log form is missing");
    await act(async () => {
      fireEvent.submit(form);
    });
    expect(posted).toEqual([]);
    await user.click(screen.getByRole("button", { name: en["log.dealNone"] }));
    options.choice_required = false;
    options.suggested_deal_id = "d1";
    await act(async () => {
      await client.invalidateQueries({ queryKey: ["activity-deal-filing"] });
    });
    await save(user);
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      deal_filing: { record_type: "contact", record_id: "p1", skip: true },
      links: [{ entity_type: "contact", entity_id: "p1" }],
    });
  });

  it("searches only this company's open deals and sends only the selected deal", async () => {
    const { posted, urls } = mount(
      { choice_required: true, company_id: "o1" },
      "d2",
    );
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText(en["log.subject"]),
      "Renewal call",
    );
    const search = await screen.findByRole("searchbox", {
      name: en["log.dealFiling"],
    });
    vi.useFakeTimers();
    try {
      fireEvent.change(search, { target: { value: "renewal" } });
      act(() => vi.advanceTimersByTime(250));
    } finally {
      vi.useRealTimers();
    }
    await user.click(
      await screen.findByRole("button", { name: "Chosen renewal" }),
    );
    await save(user);
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      links: [
        { entity_type: "contact", entity_id: "p1" },
        { entity_type: "deal", entity_id: "d2" },
      ],
      deal_filing: { record_type: "contact", record_id: "p1", skip: false },
    });
    const query = urls.find((url) => url.pathname.endsWith("/deals"));
    expect(query?.searchParams.get("company_id")).toBe("o1");
    expect(query?.searchParams.get("status")).toBe("open");
  });

  it("keeps a call off deals when the company has none open", async () => {
    const { posted } = mount({ choice_required: false, company_id: "o1" });
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText(en["log.subject"]),
      "Account call",
    );
    await save(user);
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      links: [{ entity_type: "contact", entity_id: "p1" }],
    });
    expect(screen.queryByText(en["log.dealChoice"])).toBeNull();
  });
});
