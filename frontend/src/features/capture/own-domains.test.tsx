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
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { OwnDomainsCard } from "./own-domains";

// Settings → Capture: the domains this installation treats as its own. Two
// lists with two different owners share ONE card — the company profile claims
// the first and this surface cannot touch them, the second is managed here — so
// each is its own named row and only the managed one carries verbs. A remove
// button beside a domain nobody can remove here is the defect that separation
// exists to prevent.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const CAPTURE_EDITOR: GrantSpec = { capture_settings: ["update"] };

type BackendOptions = {
  anchors?: string[];
  domains?: {
    domain: string;
    source: "admin" | "mailbox";
    verified: boolean;
  }[];
  /** A problem body every POST is refused with. */
  postRefusal?: { status: number; detail: string };
};

// backendFor answers /me with the given grants and /capture/email-domains with
// the given lists, capturing any POST body so the wire shape is assertable.
function backendFor(allow: GrantSpec, opts: BackendOptions = {}) {
  const state = {
    data: opts.domains ?? [],
    anchor_domains: opts.anchors ?? [],
  };
  let capturedPost: unknown = null;
  const deleted: string[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(req.url, "http://localhost");
      if (url.pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (req.method === "DELETE") {
        const domain = decodeURIComponent(url.pathname.split("/").pop() ?? "");
        deleted.push(domain);
        state.data = state.data.filter((row) => row.domain !== domain);
        return new Response(null, { status: 204 });
      }
      if (url.pathname.endsWith("/capture/email-domains")) {
        if (req.method === "POST") {
          const body: { domain: string } = await req.json();
          capturedPost = body;
          if (opts.postRefusal) {
            return new Response(JSON.stringify(opts.postRefusal), {
              status: opts.postRefusal.status,
              headers: { "content-type": "application/problem+json" },
            });
          }
          // Adding a candidate confirms it, as the server does.
          state.data = state.data.map((row) =>
            row.domain === body.domain
              ? { ...row, source: "admin", verified: true }
              : row,
          );
          return jsonResponse(
            { domain: "brandt.de", source: "admin", verified: true },
            201,
          );
        }
        return jsonResponse(state);
      }
      throw new Error(`unexpected request: ${req.method} ${url.pathname}`);
    },
  );
  return { fetchMock, post: () => capturedPost, deleted };
}

function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {node}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("OwnDomainsCard", () => {
  it("keeps the company-claimed domains and the managed ones in separate groups", async () => {
    const { fetchMock } = backendFor(CAPTURE_EDITOR, {
      anchors: ["brandt-automotive.de"],
      domains: [{ domain: "brandt.de", source: "admin", verified: true }],
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<OwnDomainsCard />);

    const company = await screen.findByTestId("own-domains-from-company");
    expect(
      screen.getByRole("heading", { name: en["ownDomains.companyTitle"] }),
    ).toBeTruthy();
    expect(within(company).getByText("brandt-automotive.de")).toBeTruthy();
    // Read-only means read-only: nothing in this list offers to change a list
    // the company profile owns.
    expect(within(company).queryByRole("button")).toBeNull();
    expect(within(company).queryByRole("textbox")).toBeNull();

    const managed = await screen.findByTestId("own-domains-list");
    expect(
      screen.getByRole("heading", { name: en["ownDomains.curatedTitle"] }),
    ).toBeTruthy();
    expect(
      within(managed).getByRole("button", { name: /remove brandt\.de/i }),
    ).toBeTruthy();
    // The company group's one verb leads to the profile that owns its rows.
    expect(
      screen
        .getByRole("link", { name: en["ownDomains.openCompany"] })
        .getAttribute("href"),
    ).toBe("/#/settings/company");
    // The add form is behind ONE verb, in the card's header rather than in a
    // row that repeats its own label — and the field is not on the card until
    // the verb is pressed, so every row stays an answer.
    expect(
      screen.getByRole("button", { name: en["ownDomains.addOpen"] }),
    ).toBeTruthy();
    expect(screen.queryByLabelText(/add own domain/i)).toBeNull();
  });

  it("shows no company group when the company profile claims no domain", async () => {
    const { fetchMock } = backendFor(CAPTURE_EDITOR);
    vi.stubGlobal("fetch", fetchMock);

    render(<OwnDomainsCard />);

    // The empty managed list still states itself; a group whose whole content
    // would be an empty read-only list says nothing worth naming.
    expect(await screen.findByTestId("own-domains-empty")).toBeTruthy();
    expect(screen.queryByTestId("own-domains-from-company")).toBeNull();
    expect(
      screen.queryByRole("heading", { name: en["ownDomains.companyTitle"] }),
    ).toBeNull();
  });

  it("adds a domain through the dialog the add verb opens", async () => {
    const user = userEvent.setup();
    const { fetchMock, post } = backendFor(CAPTURE_EDITOR);
    vi.stubGlobal("fetch", fetchMock);

    render(<OwnDomainsCard />);

    await screen.findByTestId("own-domains-empty");
    await user.click(
      screen.getByRole("button", { name: en["ownDomains.addOpen"] }),
    );
    const dialog = screen.getByRole("dialog");
    // What a registration costs is said where it is made.
    expect(
      within(dialog).getByText(en["ownDomains.irreversible"]),
    ).toBeTruthy();
    await user.type(
      within(dialog).getByLabelText(/add own domain/i),
      "brandt.de",
    );
    await user.click(within(dialog).getByRole("button", { name: /^add$/i }));

    await waitFor(() => expect(post()).toEqual({ domain: "brandt.de" }));
    // The dialog is the form; a write that landed leaves nothing to submit
    // again, and the card behind it is what reports the new list.
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("refuses both verbs for a role that cannot change the list, and says why once", async () => {
    const { fetchMock } = backendFor(
      {},
      { domains: [{ domain: "brandt.de", source: "admin", verified: true }] },
    );
    vi.stubGlobal("fetch", fetchMock);

    render(<OwnDomainsCard />);

    // Refused, not hidden: a rep who cannot find a thread should be able to
    // see which domains explain that.
    const remove = await screen.findByRole("button", {
      name: /remove brandt\.de/i,
    });
    const add = screen.getByRole("button", { name: en["ownDomains.addOpen"] });
    expect(remove.hasAttribute("disabled")).toBe(true);
    expect(add.hasAttribute("disabled")).toBe(true);
    // One sentence, and both refused verbs point at it — a reason a screen
    // reader only reaches by wandering into the paragraph is no reason at all.
    const denial = screen.getByText(
      /only an administrator or operations user/i,
    );
    expect(remove.getAttribute("aria-describedby")).toBe(denial.id);
    expect(add.getAttribute("aria-describedby")).toBe(denial.id);
  });

  // Adding a confirmed domain again restores it, so its removal runs at once
  // and offers Undo instead of asking first.
  it("removes a confirmed domain at once and puts it back on Undo", async () => {
    const user = userEvent.setup();
    const { fetchMock, post, deleted } = backendFor(CAPTURE_EDITOR, {
      domains: [{ domain: "brandt.de", source: "admin", verified: true }],
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<OwnDomainsCard />);

    await user.click(
      await screen.findByRole("button", { name: /remove brandt\.de/i }),
    );
    await waitFor(() => expect(deleted).toEqual(["brandt.de"]));
    await user.click(
      await screen.findByRole("button", { name: en["common.undo"] }),
    );
    await waitFor(() => expect(post()).toEqual({ domain: "brandt.de" }));
  });

  // The removed row takes its verb with it, so focus moves on rather than
  // dropping to the page.
  it("hands focus to the next domain's verb, then to the add verb", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backendFor(CAPTURE_EDITOR, {
      domains: [
        { domain: "brandt.de", source: "admin", verified: true },
        { domain: "brandt.com", source: "admin", verified: true },
      ],
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<OwnDomainsCard />);

    await user.click(
      await screen.findByRole("button", { name: /remove brandt\.de/i }),
    );
    const next = screen.getByRole("button", { name: /remove brandt\.com/i });
    await waitFor(() => expect(document.activeElement).toBe(next));

    await user.click(next);
    const add = screen.getByRole("button", { name: en["ownDomains.addOpen"] });
    await waitFor(() => expect(document.activeElement).toBe(add));
  });

  // Adding a candidate again would confirm it, which is not the inverse.
  it("offers no Undo for a candidate it removed", async () => {
    const user = userEvent.setup();
    const { fetchMock, deleted } = backendFor(CAPTURE_EDITOR, {
      domains: [{ domain: "seen.de", source: "mailbox", verified: false }],
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<OwnDomainsCard />);

    expect(await screen.findByText(en["ownDomains.candidate"])).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /remove seen\.de/i }));
    await waitFor(() => expect(deleted).toEqual(["seen.de"]));
    expect(
      await screen.findByText(
        en["settings.removedItem"].replace("{name}", "seen.de"),
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["common.undo"] }),
    ).toBeNull();
  });

  // Confirming is adding the domain again, so the row's verb sends the same
  // write the add dialog does.
  it("confirms a candidate in place", async () => {
    const user = userEvent.setup();
    const { fetchMock, post } = backendFor(CAPTURE_EDITOR, {
      domains: [{ domain: "seen.de", source: "mailbox", verified: false }],
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<OwnDomainsCard />);

    expect(
      await screen.findByText(en["ownDomains.candidateNote"]),
    ).toBeTruthy();
    await user.click(
      screen.getByRole("button", {
        name: en["ownDomains.confirmLabel"].replace("{domain}", "seen.de"),
      }),
    );
    await waitFor(() => expect(post()).toEqual({ domain: "seen.de" }));
    // The confirmed row drops its mark and its confirm verb, and focus stays
    // in the row on the verb that is left.
    const remove = screen.getByRole("button", { name: /remove seen\.de/i });
    await waitFor(() => expect(document.activeElement).toBe(remove));
    expect(screen.queryByText(en["ownDomains.candidate"])).toBeNull();
  });

  it("says on the row why a confirm was refused", async () => {
    const user = userEvent.setup();
    const { fetchMock, post } = backendFor(CAPTURE_EDITOR, {
      domains: [{ domain: "seen.de", source: "mailbox", verified: false }],
      postRefusal: { status: 422, detail: "seen.de cannot be confirmed now" },
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<OwnDomainsCard />);

    await user.click(
      await screen.findByRole("button", {
        name: en["ownDomains.confirmLabel"].replace("{domain}", "seen.de"),
      }),
    );
    await waitFor(() => expect(post()).toEqual({ domain: "seen.de" }));
    const managed = screen.getByTestId("own-domains-list");
    expect((await within(managed).findByRole("alert")).textContent).toContain(
      "seen.de cannot be confirmed now",
    );
    expect(screen.getByText(en["ownDomains.candidate"])).toBeTruthy();
  });

  it("refuses confirming a candidate for a role that cannot change the list", async () => {
    const { fetchMock } = backendFor(
      {},
      { domains: [{ domain: "seen.de", source: "mailbox", verified: false }] },
    );
    vi.stubGlobal("fetch", fetchMock);

    render(<OwnDomainsCard />);

    const confirm = await screen.findByRole("button", {
      name: en["ownDomains.confirmLabel"].replace("{domain}", "seen.de"),
    });
    expect(confirm.hasAttribute("disabled")).toBe(true);
  });

  it("marks only an unconfirmed domain, since confirmed is the normal state", async () => {
    const { fetchMock } = backendFor(CAPTURE_EDITOR, {
      domains: [
        { domain: "brandt.de", source: "admin", verified: true },
        { domain: "seen.de", source: "mailbox", verified: false },
      ],
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<OwnDomainsCard />);

    const managed = await screen.findByTestId("own-domains-list");
    expect(
      within(managed).getAllByText(en["ownDomains.candidate"]),
    ).toHaveLength(1);
  });
});
