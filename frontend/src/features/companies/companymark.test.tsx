/** @vitest-environment happy-dom */
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
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { CompanyMark } from "./companymark";

type CompanyProfile = components["schemas"]["CompanyProfile"];

const COMPANY = "00000000-0000-4000-8000-000000000010";
const LOGO = `/v1/companies/${COMPANY}/logo`;
const ICON = `/v1/companies/${COMPANY}/logo/icon`;

const WITHOUT_MARK: CompanyProfile = {
  company_id: COMPANY,
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
  expect(screen.getByRole("button", { name: "Add wide logo" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Add square icon" })).toBeTruthy();
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
    screen.getByRole("button", { name: "Replace wide logo" }),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Remove wide logo" })).toBeTruthy();

  const icon = slot("Square icon");
  expect(icon.queryByRole("img", { name: "Acme GmbH" })).toBeNull();
  expect(icon.getByText("AG")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Add square icon" })).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "Remove square icon" }),
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
    screen.getByRole("button", { name: "Replace square icon" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Remove square icon" }),
  ).toBeTruthy();
});

// The verb under a tile opens that tile's own picker, never the other slot's.
it.each([
  { verb: "Replace wide logo", slotName: "Wide logo" },
  { verb: "Replace square icon", slotName: "Square icon" },
] as const)(
  "$verb opens the picker of its own tile",
  async ({ verb, slotName }) => {
    const user = userEvent.setup();
    mark(WITH_BOTH);
    const opened: HTMLInputElement[] = [];
    const click = vi
      .spyOn(HTMLInputElement.prototype, "click")
      .mockImplementation(function (this: HTMLInputElement) {
        opened.push(this);
      });

    await user.click(screen.getByRole("button", { name: verb }));

    expect(opened).toEqual([slot(slotName).getByLabelText(slotName)]);
    click.mockRestore();
  },
);

// One caption describes both files, tied to both tiles, so a keyboard reader
// on either hears the format it takes.
it("describes both tiles with one format line", () => {
  mark(WITHOUT_MARK);
  const wide = slot("Wide logo").getByLabelText("Wide logo");
  const square = slot("Square icon").getByLabelText("Square icon");
  const hint = wide.getAttribute("aria-describedby");
  expect(hint).toBeTruthy();
  expect(square.getAttribute("aria-describedby")).toBe(hint);
  expect(document.getElementById(hint ?? "")?.textContent).toMatch(
    /SVG or transparent PNG under 5 MB/,
  );
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
  { slotName: "Wide logo", path: "/v1/company/logo" },
  { slotName: "Square icon", path: "/v1/company/logo/icon" },
] as const)(
  "sends $slotName to the upload route that owns it",
  async ({ slotName, path }) => {
    const user = userEvent.setup();
    const fetchStub = stubFetch(WITH_BOTH);
    mark(WITHOUT_MARK);

    // The tile is the target: no verb has to be pressed before a file lands.
    const chosen = new File(["not really a png"], "acme-logo.png", {
      type: "image/png",
    });
    await user.upload(slot(slotName).getByLabelText(slotName), chosen);

    await waitFor(() => expect(fetchStub).toHaveBeenCalled());
    const [sent, init] = fetchStub.mock.calls[0];
    expect(sent).toBe(path);
    expect(init?.method).toBe("POST");
    // The part's NAME is the contract's, and a body that spells it differently
    // reaches a server that answers 422 for a file the contact did choose.
    const body = init?.body as FormData;
    expect((body.get("file") as File).name).toBe("acme-logo.png");
  },
);

it.each([
  { verb: "Remove wide logo", path: "/v1/company/logo" },
  { verb: "Remove square icon", path: "/v1/company/logo/icon" },
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

const UNSUPPORTED = {
  type: "about:blank",
  title: "Unsupported media type",
  status: 415,
  code: "unsupported_media_type",
  detail: "the upload is not an image this server can read",
};

function refuseUploads(
  problem: Readonly<Record<string, unknown> & { status: number }> = UNSUPPORTED,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(problem, { status: problem.status })),
  );
}

const NOT_AN_IMAGE = () =>
  new File(["truncated"], "half-a-logo.png", { type: "image/png" });

// The refusal names its slot and is tied to that slot's control only.
// A notice under both slots would accuse a mark never touched.
it("names the refused slot and describes only that slot's control", async () => {
  const user = userEvent.setup();
  refuseUploads();
  mark(WITHOUT_MARK);

  await user.upload(
    slot("Square icon").getByLabelText("Square icon"),
    NOT_AN_IMAGE(),
  );

  const alert = await screen.findByRole("alert");
  expect(within(alert).getByText("Square icon not uploaded")).toBeTruthy();
  expect(
    within(alert).getByText("Use a PNG, JPEG, GIF, WebP, ICO or SVG file."),
  ).toBeTruthy();
  const described = slot("Square icon")
    .getByLabelText("Square icon")
    .getAttribute("aria-describedby");
  expect(described?.split(" ")).toContain(alert.parentElement?.id);
  expect(
    slot("Wide logo")
      .getByLabelText("Wide logo")
      .getAttribute("aria-describedby"),
  ).not.toContain(alert.parentElement?.id ?? "none");
});

// The body httperr.BodyTooLargeRefusal writes: a code the server never sends
// would pass on a dead entry.
it("says what to do when the file is over the upload limit", async () => {
  const user = userEvent.setup();
  refuseUploads({
    type: "about:blank",
    title: "Request Entity Too Large",
    status: 413,
    code: "body_too_large",
    detail: "the upload exceeds the 5 MB limit",
  });
  mark(WITHOUT_MARK);

  await user.upload(
    slot("Wide logo").getByLabelText("Wide logo"),
    NOT_AN_IMAGE(),
  );

  const alert = await screen.findByRole("alert");
  expect(within(alert).getByText("Wide logo not uploaded")).toBeTruthy();
  expect(within(alert).getByText("Use a smaller file.")).toBeTruthy();
  expect(within(alert).queryByText(/exceeds/)).toBeNull();
});

// A notice inside a slot widens it and wraps the other tile onto its own row.
it("draws the refusals below the tiles, wide first, never inside a slot", async () => {
  const user = userEvent.setup();
  refuseUploads();
  mark(WITHOUT_MARK);

  await user.upload(
    slot("Square icon").getByLabelText("Square icon"),
    NOT_AN_IMAGE(),
  );
  await user.upload(
    slot("Wide logo").getByLabelText("Wide logo"),
    NOT_AN_IMAGE(),
  );

  const alerts = await screen.findAllByRole("alert");
  expect(alerts).toHaveLength(2);
  expect(
    alerts.map((alert) => alert.querySelector(".callout-title")?.textContent),
  ).toEqual(["Wide logo not uploaded", "Square icon not uploaded"]);
  for (const alert of alerts) {
    expect(alert.closest(".brand-slot")).toBeNull();
  }
  const slots = alerts[0].closest(".brand-refusals")?.previousElementSibling;
  expect(slots?.className).toBe("brand-slots");
  expect(slots?.querySelectorAll("section")).toHaveLength(2);
});

// The removal is judged by the server too, and its refusal lands in the same
// place: a contact who pressed Remove and saw nothing change would press it
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
          title: "Service Unavailable",
          status: 503,
          code: "write_contention",
          detail:
            "this write met a concurrent write to the same records and was rolled back to let the other finish. Nothing in the request is wrong and nothing was written; send it again.",
        },
        { status: 503 },
      ),
    ),
  );
  mark(WITH_MARK);

  await user.click(screen.getByRole("button", { name: "Remove wide logo" }));

  expect(await screen.findByText("Wide logo not removed")).toBeTruthy();
  expect(await screen.findByText(/met a concurrent write/)).toBeTruthy();
});

it("tells the admin to outline the text when an SVG draws its words as text", async () => {
  const user = userEvent.setup();
  refuseUploads({
    type: "about:blank",
    title: "Unprocessable Entity",
    status: 422,
    code: "svg_text_not_outlined",
    detail: "the SVG draws its words as text, which this server cannot render",
  });
  mark(WITHOUT_MARK);

  await user.upload(
    slot("Wide logo").getByLabelText("Wide logo"),
    new File(["<svg/>"], "acme.svg", { type: "image/svg+xml" }),
  );

  const alert = await screen.findByRole("alert");
  expect(
    within(alert).getByText(
      "This SVG holds its words as text, which cannot be drawn. Convert the text to outlines in your design tool, or upload a PNG.",
    ),
  ).toBeTruthy();
  expect(within(alert).queryByText(/cannot render/)).toBeNull();
});

it("tells the admin to export a PNG when an SVG has parts that cannot be drawn", async () => {
  const user = userEvent.setup();
  refuseUploads({
    type: "about:blank",
    title: "Unprocessable Entity",
    status: 422,
    code: "svg_not_drawable",
    detail: "the SVG has parts this server cannot draw",
  });
  mark(WITHOUT_MARK);

  await user.upload(
    slot("Wide logo").getByLabelText("Wide logo"),
    new File(["<svg/>"], "acme.svg", { type: "image/svg+xml" }),
  );

  const alert = await screen.findByRole("alert");
  expect(
    within(alert).getByText(
      "This SVG has parts that cannot be drawn here, or draws nothing. Export the logo as a PNG from your design tool.",
    ),
  ).toBeTruthy();
});
