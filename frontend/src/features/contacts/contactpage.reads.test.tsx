/** @vitest-environment happy-dom */
import {
  focusManager,
  type QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { createQueryClient } from "../../app/queryclient";
import { RecordShell } from "../../app/testing/recordshell.testkit";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
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
import { view as liveView } from "./contactpage.testkit";

// The page's two reads, the row and the 360, against each other: which one
// decides that the page opens, opens archived, or cannot open.

afterEach(cleanup);

// The application's own client, so a record read is live as it is in the
// product: re-read on a return to the tab.
function mount(routes: RouteMap, client: QueryClient = createQueryClient()) {
  installFetchStub(routes);
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>
          <ContactPageV2 id="p-1" tab="overview" />
        </RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
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

      expect(
        await screen.findByText(en["contact.page.notOpened"]),
      ).toBeTruthy();
    });
  }

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

  it("keeps the page while a refused 360 waits on the row being re-read", async () => {
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
    expect(screen.queryByText(en["contact.page.notOpened"])).toBeNull();
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
