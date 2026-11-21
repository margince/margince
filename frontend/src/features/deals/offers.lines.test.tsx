/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { installFetchStub, meRoute } from "../../testing/story-utils";
import { jsonResponse, type Offer, offer, render } from "./deals.testkit";
import { type OfferLineItem, OfferScreen } from "./offers";

// The draft offer's line editor as a user drives it. It covers what each row
// lets a reader see and change, and what the page shows when the server says no.

afterEach(cleanup);

function renderOffer() {
  render(<OfferScreen id="o-1" />);
}

const line1: OfferLineItem = {
  id: "li-1",
  position: 1,
  product_id: null,
  description: "Consulting hours",
  unit: "hour",
  quantity: 1,
  unit_price_minor: 1000,
  discount_pct: 0,
  tax_rate: 19,
  line_net_minor: 1000,
  line_tax_minor: 190,
  line_total_minor: 1190,
  evidence: null,
  price_grounded: true,
  billing_model: null,
  billing_interval_months: null,
  interval_count: null,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const line2 = { ...line1, id: "li-2", position: 2, description: "Workshop" };

function draft(lines: OfferLineItem[]): Offer {
  return offer({
    id: "o-1",
    deal_id: "d-1",
    offer_number: "ANG-2026-0007",
    net_minor: 2000,
    tax_minor: 380,
    gross_minor: 2380,
    line_items: lines,
    version: 3,
  });
}

type Call = { url: string; body: unknown };
type Answer = { status: number; body: unknown };

// GETs read the offer; each line write is recorded and answered by `answer`.
function serve(draftOffer: Offer, answer: (call: Call) => Answer): Call[] {
  const calls: Call[] = [];
  const write = (body: unknown, url: URL) => {
    const call = { url: url.pathname, body };
    calls.push(call);
    const reply = answer(call);
    return jsonResponse(reply.body, reply.status);
  };
  const lineWrites = draftOffer.line_items.flatMap(
    (line): [string, typeof write][] => [
      [`PATCH /offers/o-1/line-items/${line.id}`, write],
      [`DELETE /offers/o-1/line-items/${line.id}`, write],
    ],
  );
  installFetchStub({
    "GET /me": meRoute({ offer: ["read", "create", "update", "delete"] }),
    "GET /offers/o-1": () => jsonResponse(draftOffer),
    "POST /offers/o-1/line-items": write,
    ...Object.fromEntries(lineWrites),
  });
  return calls;
}

describe("the Add line form", () => {
  it("starts the next line unclassified once a recurring line is added", async () => {
    const calls = serve(draft([line1]), () => ({
      status: 201,
      body: draft([line1]),
    }));
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    const billing = () => screen.getByRole("combobox", { name: "Billing" });
    await pickOption(user, billing(), "Recurring");
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Billing period" }),
      "Yearly",
    );
    await user.type(screen.getByRole("spinbutton", { name: "Periods" }), "2");
    await user.type(screen.getByTestId("new-line-description"), "Support");
    await user.type(screen.getByTestId("new-line-quantity"), "1");
    await user.type(screen.getByTestId("new-line-unit-price"), "90");
    await user.click(screen.getByTestId("add-line"));

    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].body).toMatchObject({
      billing_model: "recurring",
      billing_interval_months: 12,
      interval_count: 2,
    });
    await waitFor(() => expect(billing().textContent).toBe("Not specified"));
    expect(
      screen.queryByRole("combobox", { name: "Billing period" }),
    ).toBeNull();
    expect(screen.queryByRole("spinbutton", { name: "Periods" })).toBeNull();
  });
});

describe("the Add line form's unit price", () => {
  it("starts empty and asks for a price before it sends a line with no product", async () => {
    const calls = serve(draft([line1]), () => ({
      status: 201,
      body: draft([line1]),
    }));
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    const price = screen.getByTestId<HTMLInputElement>("new-line-unit-price");
    expect(price.value).toBe("");
    await user.type(screen.getByTestId("new-line-description"), "Free line");
    await user.type(screen.getByTestId("new-line-quantity"), "1");
    await user.click(screen.getByTestId("add-line"));
    expect(
      await screen.findByText("Enter a unit price or pick a product."),
    ).toBeTruthy();
    expect(calls).toHaveLength(0);

    await user.type(price, "0");
    await user.click(screen.getByTestId("add-line"));
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].body).toMatchObject({ unit_price_minor: 0 });
  });

  it("sends nothing while the price box holds an amount it cannot use", async () => {
    const calls = serve(draft([line1]), () => ({
      status: 201,
      body: draft([line1]),
    }));
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    await user.type(screen.getByTestId("new-line-description"), "Hours");
    await user.type(screen.getByTestId("new-line-quantity"), "1");
    await user.type(screen.getByTestId("new-line-unit-price"), "9.999");
    await user.click(screen.getByTestId("add-line"));
    expect(
      await screen.findByText("This amount has more decimals than EUR allows."),
    ).toBeTruthy();
    expect(calls).toHaveLength(0);
  });
});

describe("a line row", () => {
  it("names each control after its column and its line", async () => {
    serve(draft([line1, line2]), () => ({ status: 500, body: {} }));
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    for (const column of [
      "Position",
      "Description",
      "Unit",
      "Quantity",
      "Unit price",
      "Discount %",
      "Tax %",
    ]) {
      expect(screen.getByLabelText(`${column}, line 2`)).toBeTruthy();
    }
    expect(screen.getByRole("button", { name: "Remove line 2" })).toBe(
      screen.getByTestId("remove-line-li-2"),
    );
  });

  it("names each line's own unit box after its line", async () => {
    const crates = { ...line1, unit: "crate" };
    serve(draft([crates, { ...line2, unit: "pallet" }]), () => ({
      status: 500,
      body: {},
    }));
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    expect(screen.getByLabelText("Other unit, line 1")).toHaveProperty(
      "value",
      "crate",
    );
    expect(screen.getByLabelText("Other unit, line 2")).toHaveProperty(
      "value",
      "pallet",
    );
  });
});

describe("a saved line edit", () => {
  // A save answers with a new line version; the row must not remount around a
  // neighbouring cell the reader is still typing in.
  it("keeps what is being typed in the same row when another cell saves", async () => {
    const saved = { ...line1, quantity: 2, line_total_minor: 2999, version: 2 };
    let answer: ((response: Response) => void) | undefined;
    installFetchStub({
      "GET /me": meRoute({ offer: ["read", "update"] }),
      "GET /offers/o-1": () => jsonResponse(draft([line1])),
      "PATCH /offers/o-1/line-items/li-1": () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    });
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");
    const quantity = screen.getByTestId("line-quantity-li-1");
    await user.clear(quantity);
    await user.type(quantity, "2");
    await user.click(screen.getByTestId("line-description-li-1"));
    await user.keyboard(" and travel");

    await waitFor(() => expect(answer).toBeDefined());
    answer?.(jsonResponse(draft([saved])));
    // The saved line's own total shows once the answer is drawn.
    expect(await screen.findByText("€29.99")).toBeTruthy();
    const description = screen.getByTestId("line-description-li-1");
    expect(description).toHaveProperty("value", "Consulting hours and travel");
    expect(document.activeElement).toBe(description);
  });
});

describe("a refused line edit", () => {
  const reason = "quantity must be greater than 0";
  const refusal = {
    status: 422,
    body: {
      title: "Unprocessable Entity",
      detail: reason,
      code: "validation_error",
    },
  };

  it("puts the saved value back and keeps the reason in that row until the line saves", async () => {
    let refuseLine1 = true;
    const saved = { ...line1, quantity: 2, version: 2 };
    const renamed = { ...line2, description: "Workshop day", version: 2 };
    serve(draft([line1, line2]), (call) => {
      if (call.url.endsWith("/li-2")) {
        return { status: 200, body: draft([line1, renamed]) };
      }
      return refuseLine1
        ? refusal
        : { status: 200, body: draft([saved, renamed]) };
    });
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");
    const quantity = () => screen.getByTestId("line-quantity-li-1");
    const row1 = () => screen.getByTestId("line-row-li-1");

    await user.clear(quantity());
    await user.type(quantity(), "0");
    await user.tab();
    expect(await within(row1()).findByText(reason)).toBeTruthy();
    expect(quantity()).toHaveProperty("value", "1");

    const description = screen.getByTestId("line-description-li-2");
    await user.type(description, " day");
    await user.tab();
    await waitFor(() =>
      expect(screen.getByTestId("line-description-li-2")).toHaveProperty(
        "value",
        "Workshop day",
      ),
    );
    expect(screen.getAllByText(reason)).toHaveLength(1);
    expect(within(row1()).getByText(reason)).toBeTruthy();

    refuseLine1 = false;
    await user.clear(quantity());
    await user.type(quantity(), "2");
    await user.tab();
    await waitFor(() => expect(screen.queryByText(reason)).toBeNull());
    expect(quantity()).toHaveProperty("value", "2");
  });

  it("shows a refused removal in its row and leaves the line in place", async () => {
    serve(draft([line1, line2]), () => refusal);
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    await user.click(screen.getByRole("button", { name: "Remove line 2" }));
    expect(
      await within(screen.getByTestId("line-row-li-2")).findByText(reason),
    ).toBeTruthy();
    expect(
      within(screen.getByTestId("line-row-li-1")).queryByText(reason),
    ).toBeNull();
  });
});

describe("a line's billing", () => {
  it("lets a recurring line with no term take its periods in its row", async () => {
    const quarterly: OfferLineItem = {
      ...line1,
      billing_model: "recurring",
      billing_interval_months: 3,
      interval_count: null,
    };
    const calls = serve(draft([quarterly]), () => ({
      status: 200,
      body: draft([quarterly]),
    }));
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    const billing = screen.getByRole("combobox", { name: "Billing, line 1" });
    expect(billing.textContent).toBe("Recurring");
    expect(
      screen.getByRole("combobox", { name: "Billing period, line 1" })
        .textContent,
    ).toBe("Quarterly");
    await user.type(
      screen.getByRole("spinbutton", { name: "Periods, line 1" }),
      "4",
    );
    await user.tab();
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].body).toEqual({
      billing_model: "recurring",
      billing_interval_months: 3,
      interval_count: 4,
    });

    await pickOption(user, billing, "One-time");
    await waitFor(() => expect(calls).toHaveLength(2));
    expect(calls[1].body).toEqual({
      billing_model: "one_time",
      billing_interval_months: null,
      interval_count: null,
    });
  });

  it("sends a line's second billing change only once the first is answered", async () => {
    const sent: unknown[] = [];
    const answers: ((response: Response) => void)[] = [];
    installFetchStub({
      "GET /me": meRoute({ offer: ["read", "update"] }),
      "GET /offers/o-1": () => jsonResponse(draft([line1])),
      "PATCH /offers/o-1/line-items/li-1": (body) => {
        sent.push(body);
        return new Promise<Response>((resolve) => answers.push(resolve));
      },
    });
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");
    const billing = () =>
      screen.getByRole("combobox", { name: "Billing, line 1" });

    await pickOption(user, billing(), "One-time");
    await pickOption(user, billing(), "Recurring");
    await waitFor(() => expect(sent).toHaveLength(1));
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Billing period, line 1" }),
      "Yearly",
    );
    expect(sent).toHaveLength(1);

    answers[0](jsonResponse(draft([{ ...line1, billing_model: "one_time" }])));
    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent[1]).toMatchObject({
      billing_model: "recurring",
      billing_interval_months: 12,
    });
  });

  it("puts a saved period back when it is emptied, since a recurring line needs one", async () => {
    const quarterly: OfferLineItem = {
      ...line1,
      billing_model: "recurring",
      billing_interval_months: 3,
      interval_count: 4,
    };
    const calls = serve(draft([quarterly]), () => ({
      status: 200,
      body: draft([quarterly]),
    }));
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");
    const period = () =>
      screen.getByRole("combobox", { name: "Billing period, line 1" });

    await pickOption(user, period(), "Not specified");
    expect(period().textContent).toBe("Quarterly");
    expect(calls).toEqual([]);
  });

  it("saves a line made recurring only once it has a period", async () => {
    const calls = serve(draft([line1]), () => ({
      status: 200,
      body: draft([line1]),
    }));
    const user = userEvent.setup();
    renderOffer();
    await screen.findByText("ANG-2026-0007");

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Billing, line 1" }),
      "Recurring",
    );
    expect(calls).toHaveLength(0);
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Billing period, line 1" }),
      "Monthly",
    );
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].body).toEqual({
      billing_model: "recurring",
      billing_interval_months: 1,
      interval_count: null,
    });
  });
});
