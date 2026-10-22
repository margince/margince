// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  type CompanyFieldName,
  type CompanyForm,
  changeDraftField,
  EMPTY_DRAFT,
} from "./onboarding";
import { CompanyConfirmCard } from "./onboarding-conversation/confirm-card";
import {
  FOUNDED,
  type Proposal,
  readWith,
  render,
  renderTriage,
  SERVICE,
  SUPPORT,
  triageDraft,
  triageProposal,
} from "./onboarding-facts.testkit";

afterEach(cleanup);

// A draft with nothing left to ask for. It is derived from the empty form, not
// listed. So the case stays "every field is answered" as the company gains
// fields, and never turns into "these seventeen are".
function everyFieldFilled(): CompanyForm {
  const values = { ...EMPTY_DRAFT.values };
  for (const field of Object.keys(values) as CompanyFieldName[]) {
    values[field] = "filled";
  }
  return values;
}

const FOUNDER: components["schemas"]["CompanySiteReadContact"] = {
  name: "Jamie Fox",
  role: "Co-founder",
  evidence_snippet: "Jamie Fox, co-founder, leads product.",
  evidence_url: "https://gradion.com/team",
};

// A stateful render, so a simulated edit changes the draft the board reads
// from. These tests check what happens on the same re-render a value lands,
// which a static fixture cannot exercise.
function TriageHarness() {
  const [draft, setDraft] = useState(triageDraft());
  return (
    <CompanyConfirmCard
      proposal={triageProposal()}
      draft={draft}
      answers={[]}
      selectedFactKeys={[]}
      setSelectedFactKeys={vi.fn()}
      missingRequired={[]}
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

describe("CompanyConfirmCard as a triage surface", () => {
  // The nav is a plain list of section names, each a jump link, with one
  // blocking count as its status. It has no per-field squares and no colour
  // legend. Advisory fields get no second count, since the named list covers
  // them.
  it("navigates by section name, badging only what blocks, not a grid of squares", () => {
    renderTriage(["icp"]);

    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    const identityLink = within(nav).getByRole("button", {
      name: /Legal company/,
    });
    // display_name is required and empty: the one field that blocks confirm,
    // and the only count this section's badge carries. The other fields are
    // empty and optional (legal_name alone is settled). They are worth a look,
    // never an obstacle, and get no count of their own.
    expect(within(identityLink).getByText("1")).toBeInTheDocument();
    expect(within(identityLink).queryByText("7")).toBeNull();
    expect(screen.queryByText("What the squares mean")).not.toBeInTheDocument();
  });

  it("lists a section's outstanding fields by name, exactly the ones its count counts", () => {
    renderTriage(["icp"]);

    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    const identityLink = within(nav).getByRole("button", {
      name: /Legal company/,
    });
    const section = identityLink.closest("li");
    if (section === null) {
      throw new Error("expected the identity section's list item to exist");
    }
    // The section holds 8 outstanding fields (legal_name is settled). The nav
    // names the first NAV_NAMED_LIMIT in row order, so the gap outranks the
    // empty fields after it. The rest go to the overflow count. Only the
    // visible label counts here; the sr-only tier has its own test.
    const named = within(section)
      .getAllByRole("button")
      .filter((button) => button !== identityLink)
      .map((button) => button.querySelector("span:not(.sr-only)")?.textContent);

    expect(named).toEqual([
      "Company name",
      "Registered address",
      "Register and VAT ID",
      "Legal form",
      "Register court",
    ]);
  });

  // A field label plus a multi-word tier phrase on one row breaks the nav's
  // layout. So the tier is sr-only text, and still reaches a screen reader.
  it("names a field with no visible status phrase, while its accessible name still carries the tier", () => {
    renderTriage(["icp"]);

    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    // The accessible name (both matchers below succeed) proves the tier
    // reaches assistive tech. The visible span is checked on its own, to prove
    // it carries the field name alone.
    const blockingItem = within(nav).getByRole("button", {
      name: /Company name.*Needed to continue/,
    });
    expect(blockingItem.querySelector("span:not(.sr-only)")).toHaveTextContent(
      "Company name",
    );
    expect(blockingItem.querySelector(".sr-only")).toHaveTextContent(
      "Needed to continue",
    );

    const advisoryItem = within(nav).getByRole("button", {
      name: /Register court.*To review/,
    });
    expect(advisoryItem.querySelector("span:not(.sr-only)")).toHaveTextContent(
      "Register court",
    );
    expect(advisoryItem.querySelector(".sr-only")).toHaveTextContent(
      "To review",
    );
  });

  // Arriving at the review is not an action the reader took. So on mount no
  // field grabs focus, and nothing scrolls to one. A jump only follows a click.
  it("leaves focus where the document put it on mount, with no field auto-focused or scrolled to", () => {
    // jsdom carries no real scrollIntoView; stubbing it on the prototype is
    // the only way to observe whether anything called it. Restored after,
    // since a global prototype patch would otherwise leak into every other
    // test in this file.
    const original = HTMLElement.prototype.scrollIntoView;
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;
    try {
      renderTriage(["icp"]);

      // display_name is required and empty, the strongest pull toward
      // focusing the first blocking row. Its input still must not hold focus.
      expect(document.activeElement).not.toBe(
        screen.getByRole("textbox", { name: /Company name/ }),
      );
      expect(document.activeElement === document.body).toBe(true);
      expect(scrollIntoView).not.toHaveBeenCalled();
    } finally {
      HTMLElement.prototype.scrollIntoView = original;
    }
  });

  it("lands focus inside the named field's row, ready to type", async () => {
    const user = userEvent.setup();
    renderTriage(["icp"]);

    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    await user.click(
      within(nav).getByRole("button", { name: /^Register court/ }),
    );

    // The jump exists so the reader can fill the field, so it lands on the
    // control rather than the row that holds it.
    const focused = document.activeElement;
    expect(focused?.closest("[data-finding-id]")).toHaveAttribute(
      "data-finding-id",
      "register_court",
    );
    expect(focused?.tagName).toMatch(/^(INPUT|TEXTAREA)$/);
  });

  // The active-section tint is only the visual mark. The semantics it reads
  // (aria-current) must land on one section alone, whatever visual treatment
  // marks it.
  it("marks exactly one section current, never zero and never more than one", () => {
    renderTriage(["icp"]);

    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    const current = within(nav)
      .getAllByRole("button")
      .filter((button) => button.getAttribute("aria-current") === "true");

    // jsdom carries no IntersectionObserver, so the board falls back to the
    // first section. The live browser starts from the same default before
    // anything has scrolled.
    expect(current).toEqual([
      within(nav).getByRole("button", { name: /Legal company/ }),
    ]);
  });

  it("shows no unresolved count for a section with nothing outstanding", () => {
    render(
      <CompanyConfirmCard
        proposal={{
          ready: true,
          fields: [],
          facts: [],
          open_questions: [],
          remaining_required_fields: [],
          draft_version: 1,
          proposal_hash: "hash",
        }}
        draft={{
          ...EMPTY_DRAFT,
          values: everyFieldFilled(),
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

    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    const salesLink = within(nav).getByRole("button", {
      name: /Positioning and sales/,
    });

    expect(within(salesLink).queryByText(/^\d+$/)).not.toBeInTheDocument();
  });

  // Contacts are a company fact (who to talk to), like an office or a service
  // line. They belong on the board, in the section nav and the group list, not
  // folded into the tail below it.
  it("promotes contacts found on the site to their own section on the board", () => {
    renderTriage([], { ...readWith([]), contacts: [FOUNDER] });

    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    expect(
      within(nav).getByRole("button", { name: /^Contacts/ }),
    ).toBeInTheDocument();

    const heading = screen.getByRole("heading", { name: "Contacts", level: 3 });
    // The section sits in the group list, where every field group does, not
    // in the reference tail further down.
    expect(heading.closest(".ob-triage-groups")).not.toBeNull();
    expect(heading.closest(".ob-triage-readmore")).toBeNull();
    const section = heading.closest("section");
    if (section === null) {
      throw new Error("expected the Contacts section to exist");
    }
    expect(within(section).getByText("Jamie Fox")).toBeInTheDocument();
    expect(within(section).getByText("Co-founder")).toBeInTheDocument();
  });

  it("says plainly when the read found no contacts, rather than a zero count", () => {
    renderTriage([], readWith([]));

    const heading = screen.getByRole("heading", { name: "Contacts", level: 3 });
    const section = heading.closest("section");
    if (section === null) {
      throw new Error("expected the Contacts section to exist");
    }
    expect(
      within(section).getByText("No contacts found on your site."),
    ).toBeInTheDocument();
    // No bare digit stands in for the sentence. The section carries no
    // outstanding count in the nav either, since nothing here is the human's
    // to resolve.
    expect(within(section).queryByText(/^\d+$/)).not.toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    const contactsLink = within(nav).getByRole("button", { name: /^Contacts/ });
    expect(within(contactsLink).queryByText(/^\d+$/)).not.toBeInTheDocument();
  });

  // The count beside Contacts or Facts means "this is what I found", never
  // "this needs you". It equals the section's own content, and no reader,
  // sighted or not, may take it for an outstanding-work count.
  it("shows how many contacts the read found in the nav, as a found quantity rather than outstanding work", () => {
    const found = [FOUNDER, { ...FOUNDER, name: "Alex Chen", role: "COO" }];
    renderTriage([], { ...readWith([]), contacts: found });

    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    const contactsLink = within(nav).getByRole("button", {
      name: /^Contacts.*2 found/,
    });
    expect(within(contactsLink).getByText("2")).toBeInTheDocument();
    // Never the blocking/advisory pill's own class: a find is not a gap.
    expect(contactsLink.querySelector(".badge-danger")).toBeNull();

    const heading = screen.getByRole("heading", { name: "Contacts", level: 3 });
    const section = heading.closest("section");
    if (section === null) {
      throw new Error("expected the Contacts section to exist");
    }
    expect(within(section).getAllByRole("listitem")).toHaveLength(found.length);
  });

  it("shows how many facts the read produced in the nav, as a found quantity rather than outstanding work", () => {
    const proposal: Proposal = {
      ...triageProposal(),
      facts: [FOUNDED, SERVICE, SUPPORT],
    };
    render(
      <CompanyConfirmCard
        proposal={proposal}
        draft={triageDraft()}
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

    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    const factsLink = within(nav).getByRole("button", {
      name: /^Facts.*3 found/,
    });
    expect(within(factsLink).getByText("3")).toBeInTheDocument();
    expect(factsLink.querySelector(".badge-danger")).toBeNull();

    // The nav's number is the same 3 the section itself renders: one
    // derivation, not a second tally kept in step by hand.
    expect(document.querySelectorAll(".ob-triage-fact").length).toBe(3);
  });

  it("keeps the read's coverage honesty in the tail, since it is provenance, not a company fact", () => {
    renderTriage([], readWith([]));

    expect(screen.getByText("Pages read and skipped")).toBeInTheDocument();
    expect(screen.getByText("Background information")).toBeInTheDocument();
    expect(
      screen.queryByText("Everything else I found"),
    ).not.toBeInTheDocument();
  });

  it("marks a required gap more urgently than a merely-empty field, in words a screen reader gets", async () => {
    const user = userEvent.setup();
    render(<TriageHarness />);

    // Both are open by default (every work row is). Collapse them to the
    // one-line shape a settled row also uses. There the mark has no label
    // beside it, so it must carry the state alone.
    const requiredRow = document.getElementById("ob-triage-row-display_name");
    const emptyRow = document.getElementById("ob-triage-row-industry");
    if (requiredRow === null || emptyRow === null) {
      throw new Error("expected both rows to exist");
    }
    await user.click(
      within(requiredRow).getByRole("button", { name: "Show less" }),
    );
    await user.click(
      within(emptyRow).getByRole("button", { name: "Show less" }),
    );

    expect(
      within(requiredRow).getByText("required, still empty"),
    ).toBeInTheDocument();
    expect(within(emptyRow).getByText("empty")).toBeInTheDocument();
    expect(
      within(emptyRow).queryByText("required, still empty"),
    ).not.toBeInTheDocument();
  });

  // An empty legal field can say why, from what the crawl's own pages show.
  // It never offers a guess dressed up as a finding.
  it("says the legal field was not published when the crawl actually fetched an imprint page", async () => {
    const user = userEvent.setup();
    renderTriage([], {
      ...readWith([]),
      pages: [
        {
          url: "https://gradion.com/impressum",
          status: "fetched",
          kind: "impressum",
        },
      ],
    });

    const row = document.getElementById("ob-triage-row-registered_address");
    if (row === null) {
      throw new Error("expected the registered_address row to exist");
    }
    await user.click(within(row).getByRole("button", { name: "Show less" }));

    expect(
      within(row).getByText(
        "Not stated on your legal notice or imprint page. Add it manually.",
      ),
    ).toBeInTheDocument();
  });

  it("says the legal field was not checked when no imprint page was crawled", async () => {
    const user = userEvent.setup();
    renderTriage([], readWith([]));

    const row = document.getElementById("ob-triage-row-registered_address");
    if (row === null) {
      throw new Error("expected the registered_address row to exist");
    }
    await user.click(within(row).getByRole("button", { name: "Show less" }));

    expect(
      within(row).getByText(
        "I found no legal notice or imprint page on your site. Add it manually.",
      ),
    ).toBeInTheDocument();
  });

  it("drops a row's mark and the section's outstanding count on the same render a value lands", async () => {
    const user = userEvent.setup();
    render(<TriageHarness />);

    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    const identityLink = within(nav).getByRole("button", {
      name: /Legal company/,
    });
    const section = identityLink.closest("li") as HTMLElement;
    const advisoryItems = () =>
      section.querySelectorAll(".ob-triage-nav-item:not([data-blocking])");
    expect(within(identityLink).getByText("1")).toBeInTheDocument();
    expect(advisoryItems()).toHaveLength(4);
    const row = document.getElementById("ob-triage-row-display_name");
    if (row === null) {
      throw new Error("expected the row to exist");
    }
    expect(within(row).getByText("required, still empty")).toBeInTheDocument();

    await user.type(
      screen.getByRole("textbox", { name: /Company name/ }),
      "Gradion",
    );

    // The blocking badge and the row's own mark are the same predicate read
    // twice: filling the one required field can only drop both together.
    // The section holds more outstanding fields than the nav names, so the
    // next advisory field takes the freed slot. The list stays full at
    // NAV_NAMED_LIMIT, with one fewer blocking row.
    expect(within(identityLink).queryByText("1")).not.toBeInTheDocument();
    expect(advisoryItems()).toHaveLength(5);
    expect(
      within(row).queryByText("required, still empty"),
    ).not.toBeInTheDocument();
  });

  // The live decision surface (DecisionScene) owns answering an open
  // question. The review card only reads extracted facts back, so it never
  // re-asks one in a second shape.
  it("renders no question panel when the proposal carries open questions", () => {
    const { container } = render(
      <CompanyConfirmCard
        proposal={{
          ...triageProposal(),
          open_questions: [
            {
              id: "clarify:registered_address:1",
              question:
                "The legal notice names more than one registered address. Which one belongs to your company?",
              field: "registered_address",
              options: [
                {
                  value: "Musterstraße 1, Berlin",
                  label: "Musterstraße 1, Berlin",
                },
                {
                  value: "Hauptstraße 5, Hamburg",
                  label: "Hauptstraße 5, Hamburg",
                },
              ],
            },
          ],
        }}
        draft={triageDraft()}
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

    expect(container.querySelector(".ob-conv-confirm-questions")).toBeNull();
    expect(
      screen.queryByText(/Which one belongs to your company/),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Musterstraße 1, Berlin" }),
    ).not.toBeInTheDocument();
    // Nothing here re-asks the question, but an unanswered one still gates
    // Continue and says so. The live DecisionScene is not the only guard
    // between an open question and confirm.
    expect(screen.getByRole("button", { name: /Continue/ })).toBeDisabled();
    expect(
      screen.getByText("A decision is still open. Answer it to continue."),
    ).toBeInTheDocument();
  });

  // The guard is built from state (proposal.open_questions vs. answers), never
  // from which scene is on screen. So the test renders the card directly, not
  // through a DecisionScene or company-act render that would hide a gap.
  it("disables continue while an unresolved open question exists, even with every required field filled", () => {
    const openQuestion = {
      id: "clarify:registered_address:1",
      question: "Which registered address belongs to your company?",
      field: "registered_address",
      options: [
        { value: "Musterstraße 1, Berlin", label: "Musterstraße 1, Berlin" },
      ],
    };
    const proposal: Proposal = {
      ...triageProposal(),
      open_questions: [openQuestion],
    };

    render(
      <CompanyConfirmCard
        proposal={proposal}
        draft={triageDraft()}
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

    expect(screen.getByRole("button", { name: /Continue/ })).toBeDisabled();

    cleanup();

    // The same proposal with the answer already landed: the question is
    // resolved and does not block, on this same render.
    render(
      <CompanyConfirmCard
        proposal={proposal}
        draft={triageDraft()}
        answers={[
          {
            clarifyId: openQuestion.id,
            field: "registered_address",
            value: "Musterstraße 1, Berlin",
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

    expect(screen.getByRole("button", { name: /Continue/ })).toBeEnabled();
  });

  // One row per RowState, so no change can drop a row's collapsed signal
  // without this test failing. Every row is collapsed first, and the check
  // reads words, not a data-state attribute or a border colour.
  it("gives every row state a non-colour signal once the row is collapsed", async () => {
    const user = userEvent.setup();
    const proposal: Proposal = {
      ready: true,
      fields: [
        {
          field: "offer_summary",
          value: "We build revenue software",
          confidence: 0.9,
          evidence_snippet: "We build revenue software for manufacturers.",
          source_url: "https://gradion.com",
        },
        {
          field: "icp",
          value: "Mid-market manufacturers",
          confidence: 0.6,
          evidence_snippet: "We serve manufacturers.",
          source_url: "https://gradion.com",
        },
        {
          field: "value_proposition",
          value: "Faster invoicing",
          confidence: 0.3,
          evidence_snippet: "Cuts invoicing time.",
          source_url: "https://gradion.com",
        },
      ],
      facts: [],
      open_questions: [],
      remaining_required_fields: ["display_name"],
      draft_version: 1,
      proposal_hash: "hash",
    };
    const draft = {
      ...EMPTY_DRAFT,
      values: {
        ...EMPTY_DRAFT.values,
        offer_summary: "We build revenue software",
        icp: "Mid-market manufacturers",
        value_proposition: "Faster invoicing",
        legal_name: "Acme GmbH",
        registered_address: "Hauptstraße 1, Berlin",
      },
      edited: new Set<CompanyFieldName>(["legal_name"]),
    };

    render(
      <CompanyConfirmCard
        proposal={proposal}
        draft={draft}
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

    // display_name: required, empty. industry: empty, optional.
    // legal_name: typed. registered_address: stored (a value with no
    // grounding at all). offer_summary/icp/value_proposition: the three
    // confidence bands.
    const signalByField: Record<string, string> = {
      display_name: "required, still empty",
      industry: "empty",
      legal_name: "Typed by you",
      registered_address: "From your profile",
      offer_summary: "high",
      icp: "medium",
      value_proposition: "low",
    };

    for (const [field, signal] of Object.entries(signalByField)) {
      const row = document.getElementById(`ob-triage-row-${field}`);
      if (row === null) {
        throw new Error(`expected a row for ${field}`);
      }
      // Every work state (required, low, empty) opens by default. Collapse it
      // back to the one-line shape this test proves.
      const showLess = within(row).queryByRole("button", {
        name: "Show less",
      });
      if (showLess !== null) {
        await user.click(showLess);
      }
      expect(within(row).getByText(signal)).toBeInTheDocument();
    }
  });

  // The densest thing the crawl produced must be visible content, grouped by
  // type, the moment the review renders. It is never behind a disclosure the
  // reader has to know to open.
  it("shows a short type's facts as open board content, never behind a shut fold", () => {
    const proposal: Proposal = {
      ...triageProposal(),
      facts: [
        {
          category: "company",
          field: "founded_year",
          value: "Founded 2011",
          value_key: "company:founded_year:2011",
          evidence_snippet: "Founded in Hamburg in 2011.",
          evidence_url: "https://gradion.com/about",
          confidence: 0.9,
        },
        {
          category: "offering",
          field: "service",
          value: "Managed Kubernetes",
          value_key: "offering:service:k8s",
          evidence_snippet: "We run Kubernetes for logistics operators.",
          evidence_url: "https://gradion.com/services",
          confidence: 0.8,
        },
      ],
    };
    render(
      <CompanyConfirmCard
        proposal={proposal}
        draft={triageDraft()}
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

    // A type this short is never worth a click, so every fold on the surface
    // is already open and the reader has nothing to discover.
    for (const fold of document.querySelectorAll("details")) {
      expect(fold.open).toBe(true);
    }
    // Both facts read straight off the page, each under its own type.
    expect(screen.getByText("Founded 2011")).toBeInTheDocument();
    expect(screen.getByText("Managed Kubernetes")).toBeInTheDocument();
    expect(screen.getByText("founded year")).toBeInTheDocument();
    expect(screen.getByText("service")).toBeInTheDocument();
    // The section is navigable like any other.
    const nav = screen.getByRole("navigation", { name: "Jump to a section" });
    expect(
      within(nav).getByRole("button", { name: /^Facts/ }),
    ).toBeInTheDocument();
  });

  // A real read of a real site returns a hundred facts and most of them are
  // one type. Left open, that type is a wall the reader scrolls past, not a
  // list they check. It also buries the short types that carry a decision. Folded, the type's name and its count are still on screen, so
  // nothing about what the read found is hidden; only the rows wait.
  it("folds a long type shut while still naming it and counting it", () => {
    const many = Array.from({ length: 12 }, (_, index) => ({
      category: "offering" as const,
      field: "service" as const,
      value: `Service ${index}`,
      value_key: `offering:service:${index}`,
      evidence_snippet: `We run service ${index}.`,
      evidence_url: "https://gradion.com/services",
      confidence: 0.8,
    }));
    render(
      <CompanyConfirmCard
        proposal={{ ...triageProposal(), facts: many }}
        draft={triageDraft()}
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

    const fold = screen.getByText("service").closest("details");
    expect(fold?.open).toBe(false);
    // The shape of what the read found survives the fold: the type is named
    // and its count is stated without opening anything.
    expect(within(fold as HTMLElement).getByText("12")).toBeInTheDocument();
  });

  // jsdom performs no layout, so it cannot assert a pixel width. It checks what
  // the width cap depends on. The meta column with the evidence chip renders
  // inside `.ob-triage-fact`, and the chip's wrapper exists to carry the cap
  // (trust.css sets `min-width: 0` and `max-width: 100%` down the flex chain).
  // Expanded, the chip shows the full snippet, never a shortened stand-in.
  it("keeps a very long evidence snippet inside the fact's own meta column rather than truncating it", async () => {
    const user = userEvent.setup();
    const longSnippet =
      "77 High Street, #09-11 High Street Plaza, Singapore (179433) Contact: info@gradion.com Authorized: Lars Jankowfsky Business Profile: 201629357M Gradion Pte Ltd trading as Gradion Consulting Group since incorporation";
    const proposal: Proposal = {
      ...triageProposal(),
      facts: [
        {
          category: "company",
          field: "contact_email",
          value: "info@gradion.com",
          value_key: "company:contact_email:info",
          evidence_snippet: longSnippet,
          evidence_url: "https://gradion.com/legal",
          confidence: 0.9,
        },
      ],
    };
    render(
      <CompanyConfirmCard
        proposal={proposal}
        draft={triageDraft()}
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

    await user.click(
      screen.getByRole("button", { name: "Evidence from gradion.com/legal" }),
    );

    const snippet = screen.getByText(`"${longSnippet}"`);
    expect(snippet).toHaveClass("evidence-chip-snippet");
    expect(snippet.closest(".ob-triage-fact-meta")).not.toBeNull();
  });

  it("still lets a fact be ticked or unticked from the new inline shape", async () => {
    const user = userEvent.setup();
    const setSelectedFactKeys = vi.fn();
    const proposal: Proposal = {
      ...triageProposal(),
      facts: [
        {
          category: "company",
          field: "founded_year",
          value: "Founded 2011",
          value_key: "company:founded_year:2011",
          evidence_snippet: "Founded in Hamburg in 2011.",
          evidence_url: "https://gradion.com/about",
          confidence: 0.9,
        },
      ],
    };
    render(
      <CompanyConfirmCard
        proposal={proposal}
        draft={triageDraft()}
        answers={[]}
        selectedFactKeys={[]}
        setSelectedFactKeys={setSelectedFactKeys}
        missingRequired={[]}
        setField={vi.fn()}
        onAcceptAll={vi.fn()}
        pending={false}
        authorizing={false}
        error={null}
      />,
    );

    await user.click(
      screen.getByRole("checkbox", { name: "Save fact: Founded 2011" }),
    );

    expect(setSelectedFactKeys).toHaveBeenCalledWith([
      "company:founded_year:2011",
    ]);
  });
});
