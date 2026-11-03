// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  changeDraftField,
  EMPTY_DRAFT,
  MAX_SELECTED_FACTS,
} from "./onboarding";
import { CompanyStep } from "./onboarding-company-form";
import { missingRequiredFields } from "./onboarding-conversation/company-proposal";
import { CompanyConfirmCard } from "./onboarding-conversation/confirm-card";
import {
  defaultSelectedFactKeys,
  type FactSelection,
  useFactSelection,
} from "./onboarding-facts";
import {
  type CompanySiteReadFact,
  FOUNDED,
  fact,
  LONG_OFFER,
  readWith,
  render,
  renderTriage,
  SERVICE,
  SUPPORT,
  triageDraft,
  triageProposal,
} from "./onboarding-facts.testkit";

afterEach(cleanup);

const INDUSTRY = fact({
  value_key: "market:served_industry:logistics",
  value: "Logistics",
  category: "market",
  field: "served_industry",
  evidence_snippet: "Trusted by freight forwarders across the EU.",
  evidence_url: "https://acme.test/industries",
  confidence: 0.62,
});

const OUTCOME = fact({
  value_key: "signal:quantified_outcome:deploys",
  value: "Cut deploy time by 40%",
  category: "signal",
  field: "quantified_outcome",
  evidence_snippet: "Deploys went from two hours to seventy minutes.",
  evidence_url: "https://acme.test/cases/freight",
  confidence: 0.34,
});

const FACTS = [FOUNDED, SERVICE, SUPPORT, INDUSTRY, OUTCOME];

// 120 facts: more than the contract's ceiling, so "select all" has to stop
// somewhere the reader can see.
const MANY = Array.from({ length: 120 }, (_, index) =>
  fact({
    value_key: `company:location:${index}`,
    value: `Office ${index}`,
    confidence: 1 - index / 1000,
  }),
);

// The ceiling as the reader is told it, spelled once: the claim is that this
// sentence reaches a screen reader exactly one time per surface stack.
const CAP_SENTENCE = "Up to 100 facts can be saved. Clear one to add another.";

// Confidence RISES with wire position, so "the first N on the wire" and "the N
// most certain" name different facts throughout: a seed that trusted the wire
// would tick the ten low-confidence facts at the head and drop the strongest
// ones off the tail. More facts than the ceiling, so the cap has to choose too.
const LOW_HEAD = 10;

const RISING = Array.from({ length: MAX_SELECTED_FACTS + 15 }, (_, index) =>
  fact({
    value_key: `company:location:${index}`,
    value: `Office ${index}`,
    // The head sits under the shared low-confidence boundary; the tail climbs
    // from it, most certain last.
    confidence:
      index < LOW_HEAD ? 0.1 + index / 100 : 0.5 + (index - LOW_HEAD) / 1000,
  }),
);

// The selection model with no UI over it, so the cap and the key list can be
// exercised without going through a disabled control.
function SelectionProbe({
  facts,
  initial = [],
}: Readonly<{ facts: readonly CompanySiteReadFact[]; initial?: string[] }>) {
  const [keys, setKeys] = useState<readonly string[]>(initial);
  const selection: FactSelection = useFactSelection(facts, keys, setKeys);
  return (
    <div>
      <output data-testid="keys">{keys.join(" ")}</output>
      <output data-testid="count">{selection.selectedCount}</output>
      <output data-testid="cap">{String(selection.atCap)}</output>
      <output data-testid="all">{String(selection.allSelected)}</output>
      <button type="button" onClick={() => selection.setAll(true)}>
        probe-all
      </button>
      <button type="button" onClick={() => selection.setAll(false)}>
        probe-none
      </button>
      {facts.map((item) => (
        <button
          key={item.value_key}
          type="button"
          onClick={() => selection.toggle(item)}
        >
          {`probe-toggle-${item.value_key}`}
        </button>
      ))}
    </div>
  );
}

function keysOf(): string[] {
  const text = screen.getByTestId("keys").textContent ?? "";
  return text === "" ? [] : text.split(" ");
}

describe("useFactSelection", () => {
  it("toggles a fact by its value_key and appends to keep the list order", async () => {
    const user = userEvent.setup();
    render(<SelectionProbe facts={FACTS} initial={[INDUSTRY.value_key]} />);

    await user.click(
      screen.getByRole("button", {
        name: `probe-toggle-${FOUNDED.value_key}`,
      }),
    );

    expect(keysOf()).toEqual([INDUSTRY.value_key, FOUNDED.value_key]);
    expect(screen.getByTestId("count")).toHaveTextContent("2");
  });

  it("removes a key on a second toggle without disturbing the rest", async () => {
    const user = userEvent.setup();
    render(
      <SelectionProbe
        facts={FACTS}
        initial={[FOUNDED.value_key, SERVICE.value_key, OUTCOME.value_key]}
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: `probe-toggle-${SERVICE.value_key}`,
      }),
    );

    expect(keysOf()).toEqual([FOUNDED.value_key, OUTCOME.value_key]);
  });

  it("reports allSelected only once every fact is in the list", async () => {
    const user = userEvent.setup();
    render(<SelectionProbe facts={[FOUNDED, SERVICE]} />);

    expect(screen.getByTestId("all")).toHaveTextContent("false");
    await user.click(screen.getByRole("button", { name: "probe-all" }));

    expect(screen.getByTestId("all")).toHaveTextContent("true");
    expect(keysOf()).toEqual([FOUNDED.value_key, SERVICE.value_key]);
  });

  it("stops setAll(true) at the contract ceiling", async () => {
    const user = userEvent.setup();
    render(<SelectionProbe facts={MANY} />);

    await user.click(screen.getByRole("button", { name: "probe-all" }));

    expect(keysOf()).toHaveLength(MAX_SELECTED_FACTS);
    expect(screen.getByTestId("cap")).toHaveTextContent("true");
    // The ceiling truncates the tail, never a fact the reader already saw.
    expect(keysOf()[0]).toBe(MANY[0].value_key);
  });

  it("refuses to add past the ceiling but still allows a removal", async () => {
    const user = userEvent.setup();
    const atCap = MANY.slice(0, MAX_SELECTED_FACTS).map(
      (item) => item.value_key,
    );
    const beyond = MANY[MAX_SELECTED_FACTS];
    render(<SelectionProbe facts={MANY} initial={atCap} />);

    await user.click(
      screen.getByRole("button", { name: `probe-toggle-${beyond.value_key}` }),
    );

    expect(keysOf()).toHaveLength(MAX_SELECTED_FACTS);
    expect(keysOf()).not.toContain(beyond.value_key);

    await user.click(
      screen.getByRole("button", {
        name: `probe-toggle-${MANY[0].value_key}`,
      }),
    );
    expect(keysOf()).toHaveLength(MAX_SELECTED_FACTS - 1);
    expect(screen.getByTestId("cap")).toHaveTextContent("false");
  });

  it("clears the whole list on setAll(false)", async () => {
    const user = userEvent.setup();
    render(
      <SelectionProbe
        facts={FACTS}
        initial={[FOUNDED.value_key, SERVICE.value_key]}
      />,
    );

    await user.click(screen.getByRole("button", { name: "probe-none" }));

    expect(keysOf()).toEqual([]);
    expect(screen.getByTestId("count")).toHaveTextContent("0");
  });
});

describe("defaultSelectedFactKeys", () => {
  it("ticks every fact above the confidence boundary, most certain first", () => {
    expect(defaultSelectedFactKeys(FACTS)).toEqual([
      FOUNDED.value_key,
      SERVICE.value_key,
      SUPPORT.value_key,
      INDUSTRY.value_key,
    ]);
  });

  it("leaves a fact the scale calls low for the reader to decide", () => {
    expect(defaultSelectedFactKeys([OUTCOME])).toEqual([]);
  });

  it("seeds by confidence rather than by wire order, and caps by confidence too", () => {
    const keys = defaultSelectedFactKeys(RISING);
    const strongest = RISING[RISING.length - 1];

    expect(keys).toHaveLength(MAX_SELECTED_FACTS);
    expect(keys[0]).toBe(strongest.value_key);
    // Not one of the low-confidence facts the wire happened to send first.
    for (const weak of RISING.slice(0, LOW_HEAD)) {
      expect(keys).not.toContain(weak.value_key);
    }
    // The ceiling drops the least certain of the eligible facts, not the tail of
    // the wire: the five weakest are out, everything above them is in.
    expect(keys).not.toContain(RISING[LOW_HEAD + 4].value_key);
    expect(keys).toContain(RISING[LOW_HEAD + 5].value_key);
  });

  it("opens a fresh read below the ceiling, with room left to add a fact", () => {
    const keys = defaultSelectedFactKeys(FACTS);
    render(<SelectionProbe facts={FACTS} initial={keys} />);

    expect(screen.getByTestId("cap")).toHaveTextContent("false");
    expect(screen.getByTestId("all")).toHaveTextContent("false");
  });
});

// The two surfaces that pick facts outside the fact table — the thread's review
// card and the edit form — go through the same selection model, so the contract
// ceiling is one rule with one explanation rather than a number re-typed per
// call site.
describe("the other fact-picking surfaces", () => {
  const AT_CAP = MANY.slice(0, MAX_SELECTED_FACTS).map(
    (item) => item.value_key,
  );
  const BEYOND = MANY[MAX_SELECTED_FACTS];

  function ConfirmHarness({ initial }: Readonly<{ initial: string[] }>) {
    const [keys, setKeys] = useState<readonly string[]>(initial);
    return (
      <>
        <CompanyConfirmCard
          proposal={{ ready: true, fields: [], facts: MANY }}
          draft={EMPTY_DRAFT}
          answers={[]}
          selectedFactKeys={keys}
          setSelectedFactKeys={setKeys}
          missingRequired={[]}
          setField={vi.fn()}
          onAcceptAll={vi.fn()}
          pending={false}
          authorizing={false}
          error={null}
        />
        <output data-testid="keys">{keys.join(" ")}</output>
      </>
    );
  }

  function FormHarness({ initial }: Readonly<{ initial: string[] }>) {
    const [keys, setKeys] = useState<readonly string[]>(initial);
    return (
      <>
        <CompanyStep
          draft={EMPTY_DRAFT}
          setField={vi.fn()}
          onPickEntity={vi.fn()}
          read={readWith(MANY)}
          missingRequired={[]}
          selectedFactKeys={keys}
          setSelectedFactKeys={setKeys}
          onFieldBlur={vi.fn()}
        />
        <output data-testid="keys">{keys.join(" ")}</output>
      </>
    );
  }

  it("refuses a fact past the ceiling on the review card, and says why", async () => {
    const user = userEvent.setup();
    render(<ConfirmHarness initial={AT_CAP} />);
    const refused = screen.getByRole("checkbox", { name: /Office 100(?!\d)/ });

    expect(refused).toBeDisabled();
    expect(screen.getByText(CAP_SENTENCE)).toBeInTheDocument();

    await user.click(refused);

    expect(keysOf()).toHaveLength(MAX_SELECTED_FACTS);
    expect(keysOf()).not.toContain(BEYOND.value_key);
  });

  it("takes the refusal back on the review card once a fact is cleared", async () => {
    const user = userEvent.setup();
    render(<ConfirmHarness initial={AT_CAP} />);

    await user.click(screen.getByRole("checkbox", { name: /Office 0(?!\d)/ }));

    expect(keysOf()).toHaveLength(MAX_SELECTED_FACTS - 1);
    expect(keysOf()).not.toContain(MANY[0].value_key);
    expect(screen.queryByText(CAP_SENTENCE)).not.toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: /Office 100(?!\d)/ }),
    ).toBeEnabled();
  });

  it("refuses a fact past the ceiling on the edit form, and says why", async () => {
    const user = userEvent.setup();
    render(<FormHarness initial={AT_CAP} />);
    const refused = screen.getByRole("checkbox", { name: /Office 100(?!\d)/ });

    expect(refused).toBeDisabled();
    expect(screen.getByText(CAP_SENTENCE)).toBeInTheDocument();

    await user.click(refused);

    expect(keysOf()).toHaveLength(MAX_SELECTED_FACTS);
    expect(keysOf()).not.toContain(BEYOND.value_key);
  });

  it("writes the form's own toggle into the same key list", async () => {
    const user = userEvent.setup();
    render(<FormHarness initial={[]} />);

    await user.click(screen.getByRole("checkbox", { name: /Office 3(?!\d)/ }));

    expect(keysOf()).toEqual([MANY[3].value_key]);
  });
});

describe("CompanyConfirmCard as a triage surface", () => {
  // "You skipped X" is a claim about the reader, so it may only name the
  // questions the reader actually declined. Picking a legal entity retires the
  // sibling questions about that entity's address and registration and then
  // fills those fields — naming them would tell someone they skipped a
  // question they were never shown, about a row that is visibly filled.
  describe("the skipped-fields tail", () => {
    function renderWithDismissal(autoResolved: boolean) {
      render(
        <CompanyConfirmCard
          proposal={triageProposal()}
          draft={triageDraft()}
          answers={[
            {
              clarifyId: "clarify:registered_address:1",
              field: "registered_address",
              value: "",
              dismissed: true,
              autoResolved,
            },
          ]}
          selectedFactKeys={[]}
          setSelectedFactKeys={vi.fn()}
          missingRequired={[]}
          setField={vi.fn()}
          onAcceptAll={vi.fn()}
          pending={false}
          authorizing={false}
          error={null}
        />,
      );
    }

    it("names a question the reader declined", () => {
      renderWithDismissal(false);
      expect(screen.getByText(/You skipped:/)).toHaveTextContent(
        "Registered address",
      );
    });

    it("stays silent about a question another answer retired", () => {
      renderWithDismissal(true);
      expect(screen.queryByText(/You skipped:/)).toBeNull();
    });
  });

  // The pinned continue bar replaced the old tally strip: progress measures
  // real completion toward being able to continue (required fields only),
  // never a count of every row the board happens to carry.
  it("measures the continue bar's progress by required fields filled, not every row on the board", () => {
    render(
      <CompanyConfirmCard
        proposal={triageProposal()}
        draft={{
          ...EMPTY_DRAFT,
          values: {
            ...EMPTY_DRAFT.values,
            offer_summary: "We do things.",
            icp: "Mid-market logistics.",
          },
        }}
        answers={[]}
        selectedFactKeys={[]}
        setSelectedFactKeys={vi.fn()}
        missingRequired={["display_name"]}
        setField={vi.fn()}
        onAcceptAll={vi.fn()}
        pending={false}
        authorizing={false}
        error={null}
      />,
    );

    // 2 of the 3 required fields (offer_summary, icp) are filled; the other
    // 13 profile fields on the board — settled or not — play no part.
    const bar = screen.getByRole("progressbar", {
      name: "Required fields completed",
    });
    expect(bar).toHaveProperty("value", 2);
    expect(bar).toHaveProperty("max", 3);
    expect(
      screen.getByText("1 field needed before you can continue"),
    ).toBeInTheDocument();
  });

  it("says plainly that nothing more is required once every required field is filled", () => {
    renderTriage([]);

    expect(
      screen.getByText("Nothing more needed. You can continue."),
    ).toBeInTheDocument();
  });

  it("counts exactly the fields the section nav marks with its blocking icon", () => {
    render(
      <CompanyConfirmCard
        proposal={triageProposal()}
        draft={{
          ...EMPTY_DRAFT,
          values: { ...EMPTY_DRAFT.values, offer_summary: "x" },
        }}
        answers={[]}
        selectedFactKeys={[]}
        setSelectedFactKeys={vi.fn()}
        missingRequired={["display_name", "icp"]}
        setField={vi.fn()}
        onAcceptAll={vi.fn()}
        pending={false}
        authorizing={false}
        error={null}
      />,
    );

    const blocking = document.querySelectorAll(
      '.ob-triage-nav-item[data-blocking="true"]',
    );
    expect(blocking).toHaveLength(2);
    expect(
      screen.getByText("2 fields needed before you can continue"),
    ).toBeInTheDocument();
  });

  // The nav's blocking cluster and the bottom bar's status sentence are now
  // the only two places the board states what is missing; a banner reciting
  // the same gap a third time above the fold was noise, not a third source
  // of truth, so its absence must never leave the surface silent about why
  // Continue is disabled.
  it("renders no blocker banner while a required field is empty, and keeps Continue disabled with the reason visible", () => {
    render(
      <CompanyConfirmCard
        proposal={triageProposal()}
        draft={EMPTY_DRAFT}
        answers={[]}
        selectedFactKeys={[]}
        setSelectedFactKeys={vi.fn()}
        missingRequired={["display_name", "offer_summary", "icp"]}
        setField={vi.fn()}
        onAcceptAll={vi.fn()}
        pending={false}
        authorizing={false}
        error={null}
      />,
    );

    expect(document.querySelector(".ob-triage-blocker")).toBeNull();
    expect(
      screen.getByText("3 fields needed before you can continue"),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
  });

  // Only display_name, offer_summary and icp ever block confirm (the server
  // 422s on exactly those three); every other empty or weakly-grounded field
  // is advisory, however many of them a section carries.
  it("presents a section as blocking exactly when it holds a required-and-empty field, never for advisory ones alone", () => {
    renderTriage(["icp"]);

    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    // Identity carries display_name (required, empty) alongside four merely
    // empty-and-optional fields — it must read as blocking.
    const identityLink = within(nav).getByRole("button", {
      name: /Legal company/,
    });
    expect(identityLink.querySelector(".badge-danger")).not.toBeNull();

    // None of buying_intents, common_objections or sales_motion is in
    // REQUIRED_FIELDS — sales can never present as blocking, however many
    // of its own fields are still empty. It still marks itself at the
    // section level (every section with outstanding work does, so the nav
    // alone tells settled, advisory and blocking apart) — but with the
    // neutral count, never the danger pill reserved for a field that
    // actually gates confirm.
    const salesLink = within(nav).getByRole("button", {
      name: /Positioning and sales/,
    });
    expect(salesLink.querySelector('[data-blocking="true"]')).toBeNull();
    expect(salesLink.querySelector(".badge-danger")).toBeNull();
    expect(salesLink.querySelector(".ob-triage-nav-advisory")).not.toBeNull();
    const salesItem = salesLink.parentElement as HTMLElement;
    const buyingIntents = within(salesItem).getByRole("button", {
      name: /Buying intents/,
    });
    expect(buyingIntents.querySelector("svg")).toBeNull();
    // No triangle glyph on an advisory row — only its own name — but the
    // tier still reaches a screen reader on the row itself.
    expect(
      within(buyingIntents).getByText("To review", {
        selector: ".sr-only",
      }),
    ).toBeInTheDocument();
  });

  it("disables continue exactly while a required field remains empty, and enables it on the same render the last one is filled", async () => {
    const user = userEvent.setup();

    function RequiredHarness() {
      const [draft, setDraft] = useState({
        ...EMPTY_DRAFT,
        values: {
          ...EMPTY_DRAFT.values,
          offer_summary: "We do things.",
          icp: "Mid-market logistics.",
        },
      });
      return (
        <CompanyConfirmCard
          proposal={triageProposal()}
          draft={draft}
          answers={[]}
          selectedFactKeys={[]}
          setSelectedFactKeys={vi.fn()}
          missingRequired={missingRequiredFields(draft.values)}
          setField={(field, value) =>
            setDraft((current) => changeDraftField(current, field, value))
          }
          onAcceptAll={vi.fn()}
          pending={false}
          authorizing={false}
          error={null}
        />
      );
    }
    render(<RequiredHarness />);

    const continueButton = screen.getByRole("button", { name: /Continue/ });
    expect(continueButton).toBeDisabled();

    await user.type(
      screen.getByRole("textbox", { name: /Company name/ }),
      "Gradion",
    );

    expect(continueButton).toBeEnabled();
  });

  it("puts a group's work rows before its settled rows in the DOM", () => {
    renderTriage(["icp"]);

    // Within the legal-identity group: display_name is required and empty,
    // legal_name is grounded and settled — the gap outranks the done row.
    // "Gradion GmbH" also names the identity card above the board; the row
    // itself is the one that is a button.
    const missingRow = screen.getByRole("textbox", { name: /Company name/ });
    const settledValue = screen.getByRole("button", { name: /Gradion GmbH/ });

    expect(
      missingRow.compareDocumentPosition(settledValue) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  // A settled row is a one-line summary that is itself the expand control;
  // the full text stays out of the DOM until the row opens, and opening it
  // yields the value editable in place.
  it("hides a prose field's full text behind its collapsed row until the reader expands it", async () => {
    const user = userEvent.setup();
    renderTriage([]);

    expect(screen.queryByText(LONG_OFFER)).not.toBeInTheDocument();
    // The row button's name carries the preview, which is what tells it
    // apart from the map square that names the same field.
    const row = screen.getByRole("button", { name: /We help logistics/ });
    expect(row).toHaveAttribute("aria-expanded", "false");

    await user.click(row);

    expect(
      screen.getByRole("textbox", { name: /Products and services/ }),
    ).toHaveValue(LONG_OFFER);
  });

  it("keeps a field's evidence snippet out of the DOM until the open row's chip is toggled", async () => {
    const user = userEvent.setup();
    renderTriage([]);

    expect(
      screen.queryByText('"Legal name on the imprint page."'),
    ).not.toBeInTheDocument();
    // Collapsed rows carry no evidence control at all; the chip appears
    // with the opened card and its snippet still waits for the toggle.
    await user.click(screen.getByRole("button", { name: /Gradion GmbH/ }));
    const toggle = screen.getByRole("button", {
      name: "Evidence from gradion.com/impressum",
    });
    expect(toggle).toHaveAttribute("aria-expanded", "false");

    await user.click(toggle);

    expect(
      screen.getByText('"Legal name on the imprint page."'),
    ).toBeInTheDocument();
  });

  // The identity card leads the surface: a name, and the fields that name
  // the business — each a jump into the row it summarizes, not another
  // form. No brand image ever renders; the contract carries none.
  it("leads with an identity card naming the company, whose values jump to their rows", async () => {
    const user = userEvent.setup();
    render(
      <CompanyConfirmCard
        proposal={triageProposal()}
        draft={{
          ...EMPTY_DRAFT,
          values: {
            ...EMPTY_DRAFT.values,
            display_name: "Gradion",
            legal_name: "Gradion GmbH",
            website: "https://gradion.com/",
            registered_address: "Musterstraße 1, Hamburg",
            industry: "Software",
          },
        }}
        answers={[]}
        selectedFactKeys={[]}
        setSelectedFactKeys={vi.fn()}
        missingRequired={[]}
        setField={vi.fn()}
        onAcceptAll={vi.fn()}
        pending={false}
        authorizing={false}
        error={null}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Gradion", level: 3 }),
    ).toBeInTheDocument();
    // The domain, not the raw URL, and no logo — the contract has none.
    expect(screen.getByText("gradion.com")).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "Musterstraße 1, Hamburg" }),
    );

    expect(document.activeElement).toHaveAttribute(
      "data-finding-id",
      "registered_address",
    );
  });

  it("says nothing about a company with no name yet", () => {
    const { container } = render(
      <CompanyConfirmCard
        proposal={triageProposal()}
        draft={EMPTY_DRAFT}
        answers={[]}
        selectedFactKeys={[]}
        setSelectedFactKeys={vi.fn()}
        missingRequired={[]}
        setField={vi.fn()}
        onAcceptAll={vi.fn()}
        pending={false}
        authorizing={false}
        error={null}
      />,
    );

    expect(container.querySelector(".ob-company-card")).toBeNull();
  });
});
