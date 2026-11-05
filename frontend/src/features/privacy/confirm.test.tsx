/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { ConfirmDetailsScreen } from "./confirm";

// The public confirm page answers TWO bodies from one endpoint, and until the
// contract carried a discriminator it published only one of them.
//
// A record link returns the contact's own record card. A consent link returns a
// single subscription question and deliberately no record fields at all — the
// mail said "confirm this subscription", and serving the card would disclose
// the name, employer, address, phone and provenance trail to whoever holds the
// link.
//
// The generated client typed every 200 from this endpoint as the record card,
// so the subscription page read `provenance` off a body with three fields and
// crashed. These tests are the pair that would have caught it.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode, locale: "en" | "de" = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const SUBSCRIPTION = {
  kind: "subscription_confirmation",
  purpose_key: "product_updates",
  purpose_label: "Product updates",
  state: "unknown",
};

const RECORD = {
  kind: "record_confirmation",
  full_name: "Anna Müller",
  title: "Head of Ops",
  company: "Buyer GmbH",
  email: "anna@example.test",
  phone: "+49 30 111",
  marketing_state: "unknown",
  provenance: [],
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the confirm page branches on what the link was for", () => {
  // THE CRASH. A subscription body carries no `provenance`, and the record
  // renderer reads `.length` off it — so before the discriminator this threw
  // rather than rendering, on the one page a marketing recipient is most
  // likely to open.
  it("renders a consent link without reading any record field", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse(SUBSCRIPTION));

    render(<ConfirmDetailsScreen token="tok-subscription" />);

    expect(await screen.findByText("Product updates")).toBeInTheDocument();
    // And NOTHING about the record: no name, no employer, no phone. This is
    // the disclosure assertion, not a layout one.
    expect(screen.queryByText("Anna Müller")).not.toBeInTheDocument();
    expect(screen.queryByText("Buyer GmbH")).not.toBeInTheDocument();
    expect(screen.queryByText("+49 30 111")).not.toBeInTheDocument();
  });

  // A subscription already granted is its own state rather than a dead form.
  // Somebody who confirmed and followed the same link again — a second click, a
  // mail client prefetching, a forwarded message — is told their answer stands.
  it("tells an already-subscribed reader that their answer is recorded", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ ...SUBSCRIPTION, state: "granted" }),
    );

    render(<ConfirmDetailsScreen token="tok-granted" />);

    expect(await screen.findByText("You are subscribed")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Confirm subscription" }),
    ).not.toBeInTheDocument();
  });

  // The other branch still works, so the discriminator did not cost the page
  // it was already serving.
  it("still renders a record link with its correctable fields", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse(RECORD));

    render(<ConfirmDetailsScreen token="tok-record" />);

    expect(await screen.findByDisplayValue("Anna Müller")).toBeInTheDocument();
    expect(screen.getByDisplayValue("anna@example.test")).toBeInTheDocument();
  });

  // AN EMPTY 5xx IS NOT A CONFIRMATION.
  //
  // openapi-fetch returns `{error: undefined}` for a non-2xx whose body is
  // empty — a proxy 502, a 500 that wrote no problem document. A guard that
  // asks only whether `error` is truthy reads those as success, and this page's
  // success state tells somebody their consent was recorded when the request
  // never reached the writer. Gated on response.ok instead.
  it("does not report a subscription as confirmed when the write failed with an empty body", async () => {
    const user = userEvent.setup();
    vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
      // openapi-fetch builds a Request object, so the method can live on the
      // input rather than on init — reading only init.method sent the POST
      // down the GET arm and the page never saw the failure at all.
      const method =
        (input instanceof Request ? input.method : init?.method) ?? "GET";
      if (method === "GET") {
        return Promise.resolve(jsonResponse(SUBSCRIPTION));
      }
      // The shape that fooled the old guard: failed, and no body to parse.
      return Promise.resolve(
        new Response(null, { status: 502, headers: { "Content-Length": "0" } }),
      );
    });

    render(<ConfirmDetailsScreen token="tok-flaky" />);

    await user.click(
      await screen.findByRole("button", { name: "Confirm subscription" }),
    );

    // Never the confirmed state, and the refusal is on screen rather than the
    // button simply re-enabling itself. The copy is the honest fallback: a 502
    // with no body genuinely reports no cause.
    await screen.findByText(/the request failed/i);
    expect(screen.queryByText("You are subscribed")).not.toBeInTheDocument();
  });

  // A token that names nothing reads the same as one that expired or was
  // already answered: the page is never an oracle for which it was.
  it("shows one invalid-link state for a 404", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ title: "Not Found" }, 404),
    );

    render(<ConfirmDetailsScreen token="tok-gone" />);

    expect(
      await screen.findByText(/link|no longer|invalid/i),
    ).toBeInTheDocument();
  });
});

// The seeded purposes are stored with English labels, so a page that printed
// the label put English inside a German or Vietnamese page.
describe("a confirm page names the seeded purposes in the reader's language", () => {
  // The heading is a label and speaks the reader's language. The sentence is
  // the consent proof. The server records the published question with the
  // stored label, so the page shows and submits that same sentence.
  it("shows and submits the same consent sentence the server records", async () => {
    const user = userEvent.setup();
    const submitted: string[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const request = input instanceof Request ? input : undefined;
      const method = request?.method ?? init?.method ?? "GET";
      if (method === "GET") {
        return jsonResponse({
          ...SUBSCRIPTION,
          purpose_key: "marketing_email",
          purpose_label: "Marketing email",
        });
      }
      const body = request ? await request.text() : String(init?.body);
      submitted.push(JSON.parse(body).marketing_wording);
      return jsonResponse({});
    });

    render(<ConfirmDetailsScreen token="tok-de-subscription" />, "de");

    expect(
      await screen.findByRole("heading", { name: "Produktneuigkeiten" }),
    ).toBeInTheDocument();
    const shown = "Bestätigen Sie, dass Sie Marketing email erhalten möchten.";
    expect(screen.getByText(shown)).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: "Abonnement bestätigen" }),
    );
    await screen.findByText("Abonnement aktiv");
    expect(submitted).toEqual([shown]);
  });

  it("names the purposes a privacy notice lists, and keeps an operator's own", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({
        kind: "privacy_notice",
        acquired_as: "referral",
        purposes: ["Business correspondence", "Partner events"],
        purpose_keys: ["business_correspondence", "partner_events"],
        rights: ["access"],
      }),
    );

    render(<ConfirmDetailsScreen token="tok-de-notice" />, "de");

    expect(
      await screen.findByText("Direkte Korrespondenz"),
    ).toBeInTheDocument();
    expect(screen.getByText("Partner events")).toBeInTheDocument();
    expect(
      screen.queryByText("Business correspondence"),
    ).not.toBeInTheDocument();
  });
});

it("lets contact details be corrected without choosing marketing", async () => {
  const user = userEvent.setup({ delay: null });
  const submitted: unknown[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const request = input instanceof Request ? input : new Request(input, init);
    if (request.method === "GET") return jsonResponse(RECORD);
    submitted.push(await request.json());
    return jsonResponse({
      corrections_staged: 1,
      marketing_recorded: false,
      erasure_requested: false,
      rights_cases: [],
    });
  });
  render(<ConfirmDetailsScreen token="correction-only" />);
  const name = await screen.findByDisplayValue("Anna Müller");
  await user.clear(name);
  await user.type(name, "Anna Meyer");
  expect(
    screen.getByText(
      "This choice is optional. You can correct your details without subscribing.",
    ),
  ).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  expect(submitted).toHaveLength(1);
  expect(submitted[0]).not.toHaveProperty("marketing_choice");
  expect(submitted[0]).not.toHaveProperty("marketing_wording");
  expect(submitted[0]).toHaveProperty("corrections", [
    { field: "full_name", value: "Anna Meyer" },
  ]);
});

describe("the page speaks the language the mail was sent in", () => {
  const NOTICE = {
    kind: "privacy_notice",
    acquired_as: "purchased_or_imported",
    rights: ["access", "erasure"],
  };

  it("renders a record link in the mail's language", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ ...RECORD, language: "vi" }),
    );

    render(<ConfirmDetailsScreen token="tok-vi" />);

    expect(
      await screen.findByText("Thông tin của quý khách"),
    ).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("vi");
    // Adopted, never stored as this reader's own pick.
    expect(globalThis.localStorage.getItem("margince.locale")).toBeNull();
  });

  it("renders a privacy notice in the mail's language", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ ...NOTICE, language: "vi" }),
    );

    render(<ConfirmDetailsScreen token="tok-notice-vi" />);

    expect(
      await screen.findByText("Thông tin đang lưu về quý khách"),
    ).toBeInTheDocument();
  });

  it("keeps the reader's language when the server names none", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse(NOTICE));

    render(<ConfirmDetailsScreen token="tok-notice" />, "de");

    expect(
      await screen.findByText("Was wir über Sie gespeichert haben"),
    ).toBeInTheDocument();
  });
});
