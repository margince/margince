/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { CompanyMark } from "./companymark";

type CompanyProfile = components["schemas"]["CompanyProfile"];

const ORG = "00000000-0000-4000-8000-000000000010";
const LOGO = `/v1/organizations/${ORG}/logo`;
const ICON = `/v1/organizations/${ORG}/logo/icon`;

const WITHOUT_MARK: CompanyProfile = {
  organization_id: ORG,
  display_name: "Acme GmbH",
};
const WITH_MARK: CompanyProfile = { ...WITHOUT_MARK, logo_url: LOGO };
const WITH_BOTH: CompanyProfile = { ...WITH_MARK, logo_icon_url: ICON };

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function Providers({ children }: Readonly<{ children: ReactNode }>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

// The control renders what it is GIVEN. The card that owns it re-renders from
// the query cache a write settles, which is why every case below asserts the
// request that was sent rather than a second render this component cannot
// produce on its own.
function mark(profile: CompanyProfile) {
  return render(
    <Providers>
      <CompanyMark profile={profile} canEdit />
    </Providers>,
  );
}

// The two slots are told apart by the heading each is named by, never by
// position: an order that changed would otherwise leave every assertion below
// passing about the wrong picture.
function slot(name: "Wide logo" | "Square icon") {
  return within(screen.getByRole("region", { name }));
}

// A company with no mark in a slot is not a broken image and not an empty slot:
// it is its own initials, and the slot says why and offers the way out.
it("stands the monogram in and offers to add each mark", () => {
  mark(WITHOUT_MARK);
  for (const name of ["Wide logo", "Square icon"] as const) {
    const field = slot(name);
    expect(field.queryByRole("img", { name: "Acme GmbH" })).toBeNull();
    expect(field.getByText("AG")).toBeTruthy();
    // Nothing to remove, so the verb that removes it is not drawn. A control
    // whose only outcome is a refusal is worse than no control.
    expect(field.queryByRole("button", { name: /^Remove/ })).toBeNull();
  }
  expect(screen.getByRole("button", { name: "Add a wide logo" })).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Add a square icon" }),
  ).toBeTruthy();
});

// Each slot answers for its own mark. A company that uploaded a wordmark and no
// badge is the ordinary case, and the icon slot must still be an invitation
// rather than look as though it already holds the wordmark.
it("draws each mark in its own slot and offers the verbs that fit it", () => {
  mark(WITH_MARK);
  expect(
    slot("Wide logo")
      .getByRole("img", { name: "Acme GmbH" })
      .querySelector("img")
      ?.getAttribute("src"),
  ).toBe(LOGO);
  expect(
    screen.getByRole("button", { name: "Replace the wide logo" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Remove the wide logo" }),
  ).toBeTruthy();

  const icon = slot("Square icon");
  expect(icon.queryByRole("img", { name: "Acme GmbH" })).toBeNull();
  expect(icon.getByText("AG")).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Add a square icon" }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "Remove the square icon" }),
  ).toBeNull();
});

it("draws both marks when the company wears both", () => {
  mark(WITH_BOTH);
  expect(
    slot("Square icon")
      .getByRole("img", { name: "Acme GmbH" })
      .querySelector("img")
      ?.getAttribute("src"),
  ).toBe(ICON);
  expect(
    screen.getByRole("button", { name: "Replace the square icon" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Remove the square icon" }),
  ).toBeTruthy();
});

// The STUB declares fetch's own parameters so the recorded calls are TYPED as
// fetch's argument tuple. The runtime records every argument either way; what a
// zero-parameter mock loses is the type, and `const [path, init]` below would
// then not compile.
function stubFetch(answer: CompanyProfile) {
  const fetchStub = vi.fn(
    async (_input: RequestInfo | URL, _init?: RequestInit) =>
      Response.json(answer, { status: 200 }),
  );
  vi.stubGlobal("fetch", fetchStub);
  return fetchStub;
}

// One case per slot, and the ROUTE is what each asserts: the two uploads share
// one decode on the server, so a slot that sent its file to the other's path
// would store the picture successfully and hang it in the wrong place.
it.each([
  {
    slotName: "Wide logo",
    add: "Add a wide logo",
    path: "/v1/company/logo",
  },
  {
    slotName: "Square icon",
    add: "Add a square icon",
    path: "/v1/company/logo/icon",
  },
] as const)(
  "sends $slotName to the upload route that owns it",
  async ({ slotName, add, path }) => {
    const user = userEvent.setup();
    const fetchStub = stubFetch(WITH_BOTH);
    mark(WITHOUT_MARK);

    await user.click(screen.getByRole("button", { name: add }));
    const chosen = new File(["not really a png"], "acme-logo.png", {
      type: "image/png",
    });
    await user.upload(slot(slotName).getByLabelText(slotName), chosen);

    await waitFor(() => expect(fetchStub).toHaveBeenCalled());
    const [sent, init] = fetchStub.mock.calls[0];
    expect(sent).toBe(path);
    expect(init?.method).toBe("POST");
    // The part's NAME is the contract's, and a body that spells it differently
    // reaches a server that answers 422 for a file the person did choose.
    const body = init?.body as FormData;
    expect((body.get("file") as File).name).toBe("acme-logo.png");
  },
);

it.each([
  { verb: "Remove the wide logo", path: "/v1/company/logo" },
  { verb: "Remove the square icon", path: "/v1/company/logo/icon" },
] as const)("removes a mark through $path", async ({ verb, path }) => {
  const user = userEvent.setup();
  const fetchStub = stubFetch(WITHOUT_MARK);
  mark(WITH_BOTH);

  await user.click(screen.getByRole("button", { name: verb }));

  await waitFor(() => expect(fetchStub).toHaveBeenCalled());
  // The typed client sends a Request, not a (url, init) pair — asserting the
  // pair here would pass on any object at all once stringified.
  const [input] = fetchStub.mock.calls[0];
  const request = input as Request;
  expect(new URL(request.url).pathname).toBe(path);
  expect(request.method).toBe("DELETE");
});

// A refusal is shown where the person is standing, and only there. The server
// is the one that judges an image: the picker's filter goes on media type and
// says nothing about whether the bytes behind it decode, so a file that passes
// the picker can still be refused — and a refusal that surfaced under BOTH
// slots would accuse a mark the person never touched.
it("shows the server's refusal under the slot that was refused", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json(
        {
          type: "about:blank",
          title: "Unsupported media type",
          status: 415,
          code: "unsupported_media_type",
          detail: "the upload is not an image this server can read",
        },
        { status: 415 },
      ),
    ),
  );
  mark(WITHOUT_MARK);

  await user.click(screen.getByRole("button", { name: "Add a square icon" }));
  await user.upload(
    slot("Square icon").getByLabelText("Square icon"),
    new File(["truncated"], "half-a-logo.png", { type: "image/png" }),
  );

  expect(
    await slot("Square icon").findByText(/not an image this server can read/),
  ).toBeTruthy();
  expect(slot("Wide logo").queryByRole("alert")).toBeNull();
});

// The removal is judged by the server too, and its refusal lands in the same
// place: a person who pressed Remove and saw nothing change would press it
// again, and a second DELETE behind a refused first one is what the guard on
// the handler exists to prevent.
it("shows the server's refusal of a removal beside the control", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json(
        {
          type: "about:blank",
          title: "Conflict",
          status: 409,
          code: "company_mark_locked",
          detail: "the mark is being replaced by another write",
        },
        { status: 409 },
      ),
    ),
  );
  mark(WITH_MARK);

  await user.click(
    screen.getByRole("button", { name: "Remove the wide logo" }),
  );

  expect(
    await screen.findByText(/being replaced by another write/),
  ).toBeTruthy();
});
