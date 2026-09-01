/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { CompanyMark } from "./companymark";

type CompanyProfile = components["schemas"]["CompanyProfile"];

const ORG = "00000000-0000-4000-8000-000000000010";
const LOGO = `/v1/organizations/${ORG}/logo`;

const WITHOUT_MARK: CompanyProfile = {
  organization_id: ORG,
  display_name: "Acme GmbH",
};
const WITH_MARK: CompanyProfile = { ...WITHOUT_MARK, logo_url: LOGO };

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

// A company with no resolved mark is not a broken image and not an empty slot:
// it is its own initials, and the row says why and offers the way out.
it("stands the monogram in and offers to add a mark", () => {
  const { container } = mark(WITHOUT_MARK);
  expect(container.querySelector(".company-mark img")).toBeNull();
  expect(container.querySelector(".company-mark .avatar")?.textContent).toBe(
    "AG",
  );
  expect(screen.getByRole("button", { name: "Add a mark" })).toBeTruthy();
  // Nothing to remove, so the verb that removes it is not drawn. A control
  // whose only outcome is a refusal is worse than no control.
  expect(screen.queryByRole("button", { name: "Remove" })).toBeNull();
});

it("draws the mark the company wears, and offers to replace or remove it", () => {
  const { container } = mark(WITH_MARK);
  expect(
    container.querySelector(".company-mark img")?.getAttribute("src"),
  ).toBe(LOGO);
  expect(screen.getByRole("button", { name: "Replace" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Remove" })).toBeTruthy();
});

// The stub declares fetch's own parameters rather than none, because what each
// case reads is the CALL: a zero-argument mock records zero-length calls, and
// the assertions below would then be reaching into an empty tuple.
function stubFetch(answer: CompanyProfile) {
  const fetchStub = vi.fn(
    async (_input: RequestInfo | URL, _init?: RequestInit) =>
      Response.json(answer, { status: 200 }),
  );
  vi.stubGlobal("fetch", fetchStub);
  return fetchStub;
}

it("sends the chosen file as the multipart part the upload route names", async () => {
  const user = userEvent.setup();
  const fetchStub = stubFetch(WITH_MARK);
  mark(WITHOUT_MARK);

  await user.click(screen.getByRole("button", { name: "Add a mark" }));
  const chosen = new File(["not really a png"], "acme-logo.png", {
    type: "image/png",
  });
  await user.upload(screen.getByLabelText("Company mark"), chosen);

  await waitFor(() => expect(fetchStub).toHaveBeenCalled());
  const [path, init] = fetchStub.mock.calls[0];
  expect(path).toBe("/v1/company/logo");
  expect(init?.method).toBe("POST");
  // The part's NAME is the contract's, and a body that spells it differently
  // reaches a server that answers 422 for a file the person did choose.
  const body = init?.body as FormData;
  expect((body.get("file") as File).name).toBe("acme-logo.png");
});

it("removes the mark through the delete the contract declares", async () => {
  const user = userEvent.setup();
  const fetchStub = stubFetch(WITHOUT_MARK);
  mark(WITH_MARK);

  await user.click(screen.getByRole("button", { name: "Remove" }));

  await waitFor(() => expect(fetchStub).toHaveBeenCalled());
  // The typed client sends a Request, not a (url, init) pair — asserting the
  // pair here would pass on any object at all once stringified.
  const [input] = fetchStub.mock.calls[0];
  const request = input as Request;
  expect(new URL(request.url).pathname).toBe("/v1/company/logo");
  expect(request.method).toBe("DELETE");
});

// A refusal is shown where the person is standing. The server is the one that
// judges an image: the picker's filter goes on media type and says nothing
// about whether the bytes behind it decode, so a file that passes the picker
// can still be refused.
it("shows the server's refusal beside the control", async () => {
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

  await user.click(screen.getByRole("button", { name: "Add a mark" }));
  await user.upload(
    screen.getByLabelText("Company mark"),
    new File(["truncated"], "half-a-logo.png", { type: "image/png" }),
  );

  expect(
    await screen.findByText(/not an image this server can read/),
  ).toBeTruthy();
});
