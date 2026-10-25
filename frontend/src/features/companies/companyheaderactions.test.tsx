/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { routeHash } from "../../lib/router";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  type ActivityDrawer,
  CompanyHeaderActions,
} from "./companyheaderactions";

// The account header's Email/Log activity/Add task strip, split out of
// companyheader.test.tsx with CompanyHeaderActions itself: the same verbs
// CompanyPrimaryActions used to draw as a labelled toolbar, now icon-first
// like every other record header's action strip.

type Company = components["schemas"]["Company"];

const COMPANY: Company = {
  writable: true,
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  owner_id: "u-owner",
  captured_by: "human:u-author",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

afterEach(() => {
  vi.unstubAllGlobals();
});

// /me answering exactly `allow`, and nothing else, for a grant read with no
// `authorization` shape beyond it: `useCanWrite` would deny regardless of
// `allow` if `/me` carried nothing more than this.
function stubGrants(allow: GrantSpec) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      const body = pathname.endsWith("/me")
        ? meFixture({ allow })
        : { data: [], page: { has_more: false, next_cursor: null } };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
}

// /me hangs, so `useCanWrite` reads `undefined` on the frames before it
// answers, for the pending-grant test below, which asserts nothing claims a
// refusal `/me` has not decided yet.
function stubMeInFlight(): Array<(response: Response) => void> {
  const answer: Array<(response: Response) => void> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((request: Request) => {
      if (new URL(request.url).pathname.endsWith("/me")) {
        return new Promise<Response>((resolve) => {
          answer.push(resolve);
        });
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            data: [],
            page: { has_more: false, next_cursor: null },
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
      );
    }),
  );
  return answer;
}

function renderInApp(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// The drawer's open state moved to the page once the daily brief's own
// leading card gained a door into it (companies.tsx); this wrapper stands in
// for that page so the click-driven assertions below still exercise the real
// open/close wiring rather than a state this test would otherwise own itself.
function ActionsHost() {
  const [drawer, setDrawer] = useState<ActivityDrawer>(null);
  return (
    <CompanyHeaderActions
      company={COMPANY}
      composerOpen={false}
      onComposerOpen={() => undefined}
      drawer={drawer}
      onDrawer={setDrawer}
    />
  );
}

function renderActions() {
  renderInApp(<ActionsHost />);
}

// The account page asks nothing before pressing Log activity/Add task, the
// same rule contactactions.tsx's identical verb keeps: `useCanWrite("activity",
// "create")` and stays visible, refused, before a read seat types a word.
// Without this, a read-only seat opened the form, typed a note, and was
// refused only on submit.
describe("Log activity and Add task, gated on the create grant", () => {
  it("refuses both without the grant, over one shared sentence", async () => {
    stubGrants({});
    const user = userEvent.setup();
    renderActions();

    const log = await screen.findByRole("button", { name: "Log activity" });
    const task = await screen.findByRole("button", { name: "Add task" });
    expect(log.hasAttribute("disabled")).toBe(true);
    expect(task.hasAttribute("disabled")).toBe(true);
    const describedBy = log.getAttribute("aria-describedby");
    expect(describedBy).toBeTruthy();
    expect(task.getAttribute("aria-describedby")).toBe(describedBy);
    expect(document.getElementById(describedBy ?? "")?.textContent).toBe(
      "You do not have permission to log activities on this record.",
    );

    // Both refused for the SAME reason, so the reader reads it once: pressing
    // one does not open a form the store would refuse anyway.
    await user.click(log);
    expect(screen.queryByRole("dialog")).toBeNull();

    // Booking needs the same grant, and says so in the booking screen's words.
    const book = screen.getByRole("button", {
      name: en["scheduling.bookContact"],
    });
    expect(book.hasAttribute("disabled")).toBe(true);
    expect(
      document.getElementById(book.getAttribute("aria-describedby") ?? "")
        ?.textContent,
    ).toBe(en["scheduling.bookRefused"]);
  });

  it("presses both, with the grant", async () => {
    stubGrants({ activity: ["create"] });
    renderActions();

    const log = await screen.findByRole("button", { name: "Log activity" });
    const task = await screen.findByRole("button", { name: "Add task" });
    expect(log.hasAttribute("disabled")).toBe(false);
    expect(task.hasAttribute("disabled")).toBe(false);
  });

  // A guard that has not answered yet refuses nothing: while /me is still in
  // flight, useCanWrite reads false the same way it would for a caller
  // without the grant, so without this, both buttons would flash the "You do
  // not have permission" sentence on every page load, before the verdict is
  // even in.
  it("stays quiet, disabled with no claimed refusal, while the grant is still in flight", async () => {
    const answer = stubMeInFlight();
    renderActions();

    const log = await screen.findByRole("button", { name: "Log activity" });
    const task = await screen.findByRole("button", { name: "Add task" });
    expect(log.hasAttribute("disabled")).toBe(true);
    expect(task.hasAttribute("disabled")).toBe(true);
    expect(
      screen
        .getByRole("button", { name: en["scheduling.bookContact"] })
        .hasAttribute("disabled"),
    ).toBe(true);
    expect(
      screen.queryByText(
        "You do not have permission to log activities on this record.",
      ),
    ).toBeNull();

    // Settled before the test ends, or the mocked request outlives it.
    for (const resolve of answer) {
      resolve(
        new Response(JSON.stringify(meFixture({ allow: {} })), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
    }
  });
});

// Outside the menu the verbs are few enough to carry a glyph, and a header
// strip of label-only buttons reads as a list of links. The glyph is an
// addition, never a replacement: the accessible name is still the verb, so
// nothing about how this button is found has moved.
it("leads Log activity and Add task with a glyph, keeping their words", async () => {
  stubGrants({ activity: ["create"] });
  renderActions();

  for (const name of [en["log.title"], en["log.addTask"]]) {
    const verb = await screen.findByRole("button", { name });
    expect(verb.textContent).toContain(name);
    // aria-hidden, so the glyph adds nothing to the name the row is found
    // by, which is what the findByRole above has already proved.
    expect(verb.querySelector("svg")?.getAttribute("aria-hidden")).toBe("true");
  }
});

// The verb bar draws each verb as a glyph over one short word, the full verb
// staying its accessible name.
it("draws the bar's verbs in the design's order with their short words", async () => {
  stubGrants({ activity: ["create"] });
  renderActions();

  const email = await screen.findByRole("button", { name: "Email" });
  expect(email.textContent?.trim()).toBe("Email");
  const log = await screen.findByRole("button", { name: en["log.title"] });
  expect(log.querySelector(".verb-word")?.textContent).toBe("Log");
  const book = screen.getByRole("button", {
    name: en["scheduling.bookContact"],
  });
  // Meet stands with Email, ahead of the two that record.
  expect(
    book.compareDocumentPosition(log) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
});

type Posted = { path: string; body: unknown };
type Asked = { posted: Posted[]; read: URL[] };

// A seat that may log, one contact at the account, and every POST kept for
// the assertions: the company verbs below reach /contacts and /activities.
function stubWritableAccount({ posted, read }: Asked) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      const { pathname } = url;
      let body: unknown = {
        data: [],
        page: { has_more: false, next_cursor: null },
      };
      if (pathname.endsWith("/me")) {
        body = meFixture({ allow: { activity: ["create"] } });
      } else if (pathname.endsWith("/contacts")) {
        read.push(url);
        body = {
          data: [{ id: "c-ines", full_name: "Ines Brandt" }],
          page: { has_more: false, next_cursor: null },
        };
      } else if (request.method === "POST") {
        const sent: unknown = await request.json();
        posted.push({ path: pathname, body: sent });
        body = { id: "a-new" };
      } else {
        read.push(url);
      }
      return new Response(JSON.stringify(body), {
        status: request.method === "POST" ? 201 : 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
}

describe("Add task writes only tasks", () => {
  it("offers no Type, names its own verb, and saves a task due on a later day", async () => {
    const posted: Posted[] = [];
    stubWritableAccount({ posted, read: [] });
    const user = userEvent.setup();
    renderActions();

    await user.click(await screen.findByRole("button", { name: "Add task" }));
    const drawer = await screen.findByRole("dialog");
    expect(
      within(drawer).getByRole("heading", { name: "Add task" }),
    ).toBeTruthy();
    expect(within(drawer).queryByLabelText(en["log.kind"])).toBeNull();
    const due = within(drawer).getByLabelText("Due date");
    await user.clear(due);
    await user.type(due, "16/03/2099");
    await user.type(within(drawer).getByLabelText("Subject *"), "Demo prep");
    await user.click(within(drawer).getByRole("button", { name: "Add task" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]?.path).toMatch(/\/activities$/);
    expect(posted[0]?.body).toMatchObject({
      kind: "task",
      subject: "Demo prep",
    });
    expect(posted[0]?.body).toHaveProperty(
      "due_at",
      expect.stringMatching(/^2099-03-1[56]T/),
    );
  });

  it("keeps Type on Log activity, and says why a later day is greyed out", async () => {
    stubWritableAccount({ posted: [], read: [] });
    const user = userEvent.setup();
    renderActions();

    await user.click(
      await screen.findByRole("button", { name: "Log activity" }),
    );
    const drawer = await screen.findByRole("dialog");
    expect(within(drawer).getByLabelText(en["log.kind"])).toBeTruthy();
    expect(within(drawer).getByText(en["log.dateHint"])).toBeTruthy();
    expect(
      within(drawer).getByRole("button", { name: en["log.save"] }),
    ).toBeTruthy();
  });
});

describe("Book a meeting from the account", () => {
  afterEach(() => {
    globalThis.location.hash = "";
  });

  it("asks which of the account's contacts, then opens that contact's booking screen", async () => {
    const read: URL[] = [];
    stubWritableAccount({ posted: [], read });
    const user = userEvent.setup();
    renderActions();

    // The picker is drawn once `/me` allows it; before that the face is a
    // disabled stand-in.
    const book = () =>
      screen.getByRole("button", { name: en["scheduling.bookContact"] });
    await waitFor(() => expect(book().hasAttribute("disabled")).toBe(false));
    await user.click(book());
    await user.type(
      await screen.findByRole("combobox", {
        name: en["scheduling.bookPickContact"],
      }),
      "Ines",
    );
    await user.click(
      await screen.findByRole("option", { name: "Ines Brandt" }),
    );

    expect(globalThis.location.hash).toBe(
      routeHash({ screen: "book", id: "contact-c-ines" }),
    );
    const contactsAsked = read.find((url) =>
      url.pathname.endsWith("/contacts"),
    );
    expect(contactsAsked?.searchParams.get("company_id")).toBe(COMPANY.id);
  });
});
