/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { CompanyHeaderActions } from "./companyheaderactions";

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

function renderActions() {
  renderInApp(
    <CompanyHeaderActions
      company={COMPANY}
      composerOpen={false}
      onComposerOpen={() => undefined}
    />,
  );
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

// The Email verb is icon-only, found by its accessible name rather than by
// visible text, the same shape contactactions.tsx draws for the identical
// verb on every other record header.
it("draws the Email verb icon-only, with its name on hover", async () => {
  stubGrants({ activity: ["create"] });
  renderActions();

  const email = await screen.findByRole("button", { name: "Email" });
  expect(email.textContent?.trim()).toBe("");
});
