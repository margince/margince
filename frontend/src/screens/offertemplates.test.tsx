/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { OfferTemplatesAdmin } from "./offertemplates";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const template = {
  id: "t-1",
  name: "Standard DE",
  locale: "de-DE",
  is_default: true,
  layout: {},
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

// One grant at a time: create, update and delete govern three different
// controls, and a fixture holding all of them cannot tell a correct binding
// from a transposed one.
const TEMPLATE_MANAGER: GrantSpec = {
  offer_template: ["read", "create", "update", "delete"],
};
const TEMPLATE_READER: GrantSpec = { offer_template: ["read"] };

// The list read, the /me snapshot the affordance gates ask, and (when a test
// supplies one) whatever the create POST should answer.
function templatesStub({
  allow,
  seat = "full",
  onPost,
  rows = [template],
}: {
  allow: GrantSpec;
  seat?: "full" | "read";
  onPost?: () => Response;
  rows?: readonly object[];
}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input instanceof Request ? input.url : input);
    const method =
      (input instanceof Request ? input.method : init?.method) ?? "GET";
    if (url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow, seat }));
    }
    if (method === "POST" && onPost) {
      return onPost();
    }
    return jsonResponse({
      data: rows,
      page: { next_cursor: null, has_more: false },
    });
  });
}

function newButton() {
  return screen.queryByRole("button", { name: "New template" });
}
function editButton() {
  return screen.queryByRole("button", { name: "Edit template" });
}
function archiveButton() {
  return screen.queryByRole("button", { name: "Archive template" });
}
function rowMenu() {
  return screen.queryByRole("button", { name: "Actions for Standard DE" });
}
async function openRowMenu() {
  await userEvent.click(
    await screen.findByRole("button", { name: "Actions for Standard DE" }),
  );
}
function postureLine() {
  return screen.queryByText(
    "Read-only. Your role cannot change offer templates.",
  );
}

describe("OfferTemplatesAdmin", () => {
  it("names a template's language in words and badges only the default", async () => {
    vi.stubGlobal(
      "fetch",
      templatesStub({
        allow: TEMPLATE_MANAGER,
        rows: [
          template,
          {
            ...template,
            id: "t-2",
            name: "Plain EN",
            locale: "en-US",
            is_default: false,
          },
        ],
      }),
    );
    render(<OfferTemplatesAdmin />);
    const standard = within(
      await screen.findByRole("row", { name: /Standard DE/ }),
    );
    expect(standard.getByText("German (DE)")).toBeTruthy();
    expect(standard.getByText("Default")).toBeTruthy();
    const plain = within(screen.getByRole("row", { name: /Plain EN/ }));
    expect(plain.getByText("English (US)")).toBeTruthy();
    expect(plain.queryByText("Default")).toBeNull();
    expect(screen.queryByText("de-DE")).toBeNull();
  });

  it("offers the language and the default in words on the New template form", async () => {
    vi.stubGlobal("fetch", templatesStub({ allow: TEMPLATE_MANAGER }));
    render(<OfferTemplatesAdmin />);
    await userEvent.click(await screen.findByTestId("new-record"));
    const form = within(await screen.findByRole("dialog"));
    const offered = async (field: RegExp) => {
      await userEvent.click(form.getByRole("combobox", { name: field }));
      const labels = within(screen.getByRole("listbox"))
        .getAllByRole("option")
        .map((option) => option.textContent);
      await userEvent.keyboard("{Escape}");
      return labels;
    };
    expect(await offered(/^Language/)).toEqual(["German (DE)", "English (US)"]);
    expect(await offered(/^Default for locale/)).toEqual(["No", "Yes"]);
  });

  it("surfaces a 409 offer_template_default_conflict detail verbatim on create", async () => {
    vi.stubGlobal(
      "fetch",
      templatesStub({
        allow: TEMPLATE_MANAGER,
        onPost: () =>
          jsonResponse(
            {
              title: "conflict",
              detail: "a default template already exists for this locale",
              code: "offer_template_default_conflict",
            },
            409,
          ),
      }),
    );
    render(<OfferTemplatesAdmin />);
    await userEvent.click(await screen.findByTestId("new-record"));
    // The column header is a sort button named "Sort by Name" now, so the
    // form field is asked for as a textbox rather than by a loose text match.
    const nameField = await screen.findByRole("textbox", { name: /Name/ });
    await userEvent.type(nameField, "Standard DE");
    await userEvent.click(screen.getByText("Create"));
    expect(
      await screen.findByText(
        "a default template already exists for this locale",
      ),
    ).toBeTruthy();
  });

  it("gives a read-only seat the rows, the posture line and no write affordance", async () => {
    vi.stubGlobal("fetch", templatesStub({ allow: TEMPLATE_READER }));
    render(<OfferTemplatesAdmin />);
    // The list itself is never withheld — every seat reads it.
    expect(await screen.findByText("Standard DE")).toBeTruthy();
    expect(postureLine()).toBeTruthy();
    expect(newButton()).toBeNull();
    expect(rowMenu()).toBeNull();
    // With both row verbs withheld the column goes too, rather than standing
    // there headed and empty.
    expect(screen.queryByRole("columnheader", { name: "Actions" })).toBeNull();
  });

  it("offers New on offer_template:create alone", async () => {
    vi.stubGlobal(
      "fetch",
      templatesStub({ allow: { offer_template: ["create"] } }),
    );
    render(<OfferTemplatesAdmin />);
    expect(
      await screen.findByRole("button", { name: "New template" }),
    ).toBeTruthy();
    expect(rowMenu()).toBeNull();
    expect(postureLine()).toBeNull();
  });

  it("offers Edit on offer_template:update alone", async () => {
    vi.stubGlobal(
      "fetch",
      templatesStub({ allow: { offer_template: ["update"] } }),
    );
    render(<OfferTemplatesAdmin />);
    await openRowMenu();
    expect(
      await screen.findByRole("button", { name: "Edit template" }),
    ).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "Actions" })).toBeTruthy();
    expect(newButton()).toBeNull();
    expect(archiveButton()).toBeNull();
    expect(postureLine()).toBeNull();
  });

  it("offers Archive on offer_template:delete alone", async () => {
    vi.stubGlobal(
      "fetch",
      templatesStub({ allow: { offer_template: ["delete"] } }),
    );
    render(<OfferTemplatesAdmin />);
    await openRowMenu();
    expect(
      await screen.findByRole("button", { name: "Archive template" }),
    ).toBeTruthy();
    expect(newButton()).toBeNull();
    expect(editButton()).toBeNull();
    expect(postureLine()).toBeNull();
  });

  it("withholds every affordance from a read seat holding all three grants", async () => {
    // The seat ceiling is clamped on the HTTP method before RBAC is consulted,
    // so the grants alone do not make a control honourable.
    vi.stubGlobal(
      "fetch",
      templatesStub({ allow: TEMPLATE_MANAGER, seat: "read" }),
    );
    render(<OfferTemplatesAdmin />);
    expect(await screen.findByText("Standard DE")).toBeTruthy();
    expect(postureLine()).toBeTruthy();
    expect(newButton()).toBeNull();
    expect(rowMenu()).toBeNull();
  });
});
