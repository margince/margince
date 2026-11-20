/** @vitest-environment happy-dom */
import {
  focusManager,
  type QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { createQueryClient } from "../../app/queryclient";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { view as liveView } from "../../testing/contactpage.testkit";
import { RecordShell } from "../../testing/recordshell.testkit";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
} from "../../testing/story-utils";
import {
  archivedContact,
  archivedContactRoutes,
  refusedRead,
} from "./contactarchived.fixtures";
import { ContactPageV2 } from "./contactpage";

// The page's two reads, the row and the 360, against each other: which one
// decides that the page opens, opens archived, or cannot open.

afterEach(cleanup);

// The application's own client, so a record read is live as it is in the
// product: re-read on a return to the tab.
function mount(routes: RouteMap, client: QueryClient = createQueryClient()) {
  installFetchStub(routes);
  const page = (id: string) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>
          <ContactPageV2 id={id} tab="overview" />
        </RecordShell>
      </LocaleProvider>
    </QueryClientProvider>
  );
  const rendered = render(page("p-1"));
  return { ...rendered, open: (id: string) => rendered.rerender(page(id)) };
}

// What the page says when its row read is refused. A 404 names no contact the
// reader may see; any other refusal keeps the page's own sentence.
function refusalOf(status: number): string {
  return status === 404
    ? en["record.gone.contact"]
    : en["contact.page.notOpened"];
}

// A row answer the spec releases itself, so it can look at the page between
// the 360's answer and the row's.
function heldRow(): {
  answer: () => Promise<Response>;
  release: (body: unknown) => void;
} {
  let release: (body: unknown) => void = () => undefined;
  const held = new Promise<Response>((settle) => {
    release = (body) => settle(jsonResponse(body));
  });
  return { answer: () => held, release: (body) => release(body) };
}

function liveRoutes(): RouteMap {
  return {
    ...archivedContactRoutes(),
    "GET /contacts/p-1": () => jsonResponse(liveView.contact),
    "GET /contacts/p-1/360": () => jsonResponse(liveView),
    "GET /contacts/p-1/brief": () =>
      jsonResponse({ contact_id: "p-1", sentences: [], generated_by: "rules" }),
    "GET /contacts/p-1/consent/guard": () =>
      jsonResponse({ contact_id: "p-1", entries: [] }),
  };
}

describe("the contact page's row read", () => {
  for (const status of [404, 500]) {
    it(`cannot open the page when the row answers ${status}, whatever the 360 says`, async () => {
      const routes = liveRoutes();
      routes["GET /contacts/p-1"] = () =>
        jsonResponse(
          { status, code: status === 404 ? "not_found" : "internal" },
          status,
        );
      // No retries: the case is the settled refusal, not the wait before it.
      const client = createQueryClient();
      client.setDefaultOptions({
        queries: { ...client.getDefaultOptions().queries, retry: false },
      });
      mount(routes, client);

      expect(await screen.findByText(refusalOf(status))).toBeTruthy();
    });
  }

  it("offers the way back to the contacts when the row is not there", async () => {
    const routes = liveRoutes();
    routes["GET /contacts/p-1"] = () =>
      jsonResponse({ status: 404, code: "not_found" }, 404);
    const client = createQueryClient();
    client.setDefaultOptions({
      queries: { ...client.getDefaultOptions().queries, retry: false },
    });
    mount(routes, client);

    await screen.findByText(en["record.gone.contact"]);
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Back to Contacts" }));
    expect(window.location.hash).toBe("#/contacts");
  });

  // The sentence replaces the page's wait, so it is announced as it arrives.
  // The way back is something to reach, not something to hear.
  it("announces that the contact is not there, with the way back outside it", async () => {
    const routes = liveRoutes();
    routes["GET /contacts/p-1"] = () =>
      jsonResponse({ status: 404, code: "not_found" }, 404);
    const client = createQueryClient();
    client.setDefaultOptions({
      queries: { ...client.getDefaultOptions().queries, retry: false },
    });
    mount(routes, client);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(en["record.gone.contact"]);
    expect(within(alert).queryByRole("button")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Back to Contacts" }),
    ).toBeTruthy();
  });

  it("asks for no 360 once the row is known archived", async () => {
    const asked: string[] = [];
    const routes = archivedContactRoutes();
    routes["GET /contacts/p-1/360"] = () => {
      asked.push("360");
      return refusedRead();
    };
    const client = createQueryClient();
    client.setQueryData(["contact", "p-1"], archivedContact);
    mount(routes, client);

    expect(await screen.findByText(en["record.archived"])).toBeTruthy();
    expect(asked).toEqual([]);
  });

  it("turns to the archived page when a return to the tab finds the row archived", async () => {
    let archived = false;
    const routes = liveRoutes();
    routes["GET /contacts/p-1"] = () =>
      jsonResponse(archived ? archivedContact : liveView.contact);
    routes["GET /contacts/p-1/360"] = () =>
      archived ? refusedRead() : jsonResponse(liveView);
    mount(routes);
    expect(
      await screen.findByRole("button", { name: en["tab.overview"] }),
    ).toBeTruthy();

    archived = true;
    act(() => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });

    expect(await screen.findByText(en["record.archived"])).toBeTruthy();
    expect(screen.queryByText(en["contact.page.notOpened"])).toBeNull();
  });

  it("withdraws the page and its actions while a refused 360 waits on the row", async () => {
    const client = createQueryClient();
    const routes = liveRoutes();
    mount(routes, client);
    expect(
      await screen.findByRole("button", { name: en["tab.overview"] }),
    ).toBeTruthy();

    const row = heldRow();
    routes["GET /contacts/p-1"] = row.answer;
    routes["GET /contacts/p-1/360"] = refusedRead;
    installFetchStub(routes);
    act(() => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });

    await waitFor(() =>
      expect(client.getQueryState(["contact360", "p-1"])?.status).toBe("error"),
    );
    expect(await screen.findByText(en["contact.page.loading"])).toBeTruthy();
    expect(screen.queryByText(en["contact.page.notOpened"])).toBeNull();
    expect(screen.queryByRole("heading")).toBeNull();
    expect(screen.queryAllByRole("button")).toHaveLength(0);
    act(() => row.release(archivedContact));
    expect(await screen.findByText(en["record.archived"])).toBeTruthy();
  });

  it("draws nothing from a 360 that answers before the row says archived", async () => {
    const client = createQueryClient();
    const row = heldRow();
    const routes = liveRoutes();
    routes["GET /contacts/p-1"] = row.answer;
    mount(routes, client);

    await waitFor(() =>
      expect(client.getQueryState(["contact360", "p-1"])?.status).toBe(
        "success",
      ),
    );
    expect(
      screen.queryByRole("button", { name: en["tab.overview"] }),
    ).toBeNull();
    act(() => row.release(archivedContact));
    expect(await screen.findByText(en["record.archived"])).toBeTruthy();
  });
});

describe("confirmed contact identity while the 360 loads", () => {
  it("shows the base name and title without starting another record read", async () => {
    const held = heldRow();
    const client = createQueryClient();
    let rows = 0;
    let views = 0;
    const contact = { ...liveView.contact, title: "Buyer" };
    mount(
      {
        ...liveRoutes(),
        "GET /contacts/p-1": () => {
          rows++;
          return jsonResponse(contact);
        },
        "GET /contacts/p-1/360": () => {
          views++;
          return held.answer();
        },
      },
      client,
    );

    expect(
      await screen.findByRole("heading", { name: contact.full_name }),
    ).toBeTruthy();
    expect(screen.getByText("Buyer")).toBeTruthy();
    expect(screen.getByText(en["contact.page.loading"])).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["tab.overview"] }),
    ).toBeNull();
    expect(screen.queryAllByRole("button")).toHaveLength(0);
    expect(rows).toBe(1);
    expect(views).toBe(1);
    act(() => held.release({ ...liveView, contact }));
    expect(
      await screen.findByRole("button", { name: en["tab.overview"] }),
    ).toBeTruthy();
    expect(screen.queryByText(en["contact.page.loading"])).toBeNull();
    expect(rows).toBe(1);
    expect(views).toBe(1);
  });

  for (const status of [403, 404, 500]) {
    it(`drops cached identity when the base fails with ${status} before the 360`, async () => {
      const held = heldRow();
      const client = createQueryClient();
      client.setDefaultOptions({
        queries: { ...client.getDefaultOptions().queries, retry: false },
      });
      client.setQueryData(["contacts", {}], {
        pages: [{ data: [liveView.contact] }],
      });
      mount(
        {
          ...liveRoutes(),
          "GET /contacts/p-1": () => jsonResponse({ status }, status),
          "GET /contacts/p-1/360": held.answer,
        },
        client,
      );
      expect(await screen.findByText(refusalOf(status))).toBeTruthy();
      expect(
        screen.queryByRole("heading", { name: liveView.contact.full_name }),
      ).toBeNull();
      expect(screen.queryByText(en["contact.page.loading"])).toBeNull();
    });
  }

  for (const status of [403, 404]) {
    it(`hides the fallback after 360 ${status} until the base resolves`, async () => {
      const row = heldRow();
      const client = createQueryClient();
      client.setQueryData(["contacts", {}], {
        pages: [{ data: [liveView.contact] }],
      });
      mount(
        {
          ...liveRoutes(),
          "GET /contacts/p-1": row.answer,
          "GET /contacts/p-1/360": () => jsonResponse({ status }, status),
        },
        client,
      );
      await waitFor(() =>
        expect(client.getQueryState(["contact360", "p-1"])?.status).toBe(
          "error",
        ),
      );
      await waitFor(() => expect(screen.queryByRole("heading")).toBeNull());
      expect(screen.getByText(en["contact.page.loading"])).toBeTruthy();
      act(() =>
        row.release(status === 404 ? archivedContact : liveView.contact),
      );
      expect(
        await screen.findByText(
          en[status === 404 ? "record.archived" : "contact.page.notOpened"],
        ),
      ).toBeTruthy();
    });

    it(`shows no cached archived row while its re-read is held, then refuses on ${status}`, async () => {
      const client = createQueryClient();
      client.setDefaultOptions({
        queries: { ...client.getDefaultOptions().queries, retry: false },
      });
      client.setQueryData(["contact", "p-1"], archivedContact, {
        updatedAt: 1,
      });
      let refuse: () => void = () => undefined;
      const held = new Promise<Response>((settle) => {
        refuse = () => settle(jsonResponse({ status }, status));
      });
      mount(
        {
          ...archivedContactRoutes(),
          "GET /contacts/p-1": () => held,
        },
        client,
      );
      expect(await screen.findByText(en["contact.page.loading"])).toBeTruthy();
      expect(screen.queryByText(en["record.archived"])).toBeNull();
      expect(screen.queryByRole("heading")).toBeNull();
      expect(screen.queryAllByRole("button")).toHaveLength(0);
      act(() => refuse());
      expect(await screen.findByText(refusalOf(status))).toBeTruthy();
      expect(screen.queryByText(en["record.archived"])).toBeNull();
      expect(
        screen.queryByRole("heading", { name: archivedContact.full_name }),
      ).toBeNull();
    });
  }

  it("opens a cached archived row once its re-read confirms it", async () => {
    const client = createQueryClient();
    client.setQueryData(["contact", "p-1"], archivedContact, { updatedAt: 1 });
    const row = heldRow();
    mount(
      { ...archivedContactRoutes(), "GET /contacts/p-1": row.answer },
      client,
    );
    expect(await screen.findByText(en["contact.page.loading"])).toBeTruthy();
    expect(screen.queryByText(en["record.archived"])).toBeNull();
    act(() => row.release(archivedContact));
    expect(await screen.findByText(en["record.archived"])).toBeTruthy();
  });

  it("does not show a previous contact when its pending 360 answers after navigation", async () => {
    const first = heldRow();
    const second = heldRow();
    const next = {
      ...liveView.contact,
      id: "p-2",
      full_name: "Second Contact",
      title: "Engineer",
    };
    const mounted = mount({
      ...liveRoutes(),
      "GET /contacts/p-1/360": first.answer,
      "GET /contacts/p-2": () => jsonResponse(next),
      "GET /contacts/p-2/360": second.answer,
      "GET /contacts/p-2/consent/guard": () =>
        jsonResponse({ contact_id: "p-2", entries: [] }),
    });
    expect(
      await screen.findByRole("heading", { name: liveView.contact.full_name }),
    ).toBeTruthy();
    mounted.open("p-2");
    expect(
      await screen.findByRole("heading", { name: next.full_name }),
    ).toBeTruthy();
    await act(async () => first.release(liveView));
    expect(
      screen.queryByRole("heading", { name: liveView.contact.full_name }),
    ).toBeNull();
    expect(screen.getByText(next.title)).toBeTruthy();
    expect(screen.getByText(en["contact.page.loading"])).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["tab.overview"] }),
    ).toBeNull();
  });
});
