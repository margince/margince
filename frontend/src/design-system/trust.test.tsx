/** @vitest-environment happy-dom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import {
  ConfidenceMeter,
  confidenceLevel,
  EvidenceChip,
  formatSourceLines,
  type Proposal,
  ProvenanceTag,
  StagedProposal,
  toEvidence,
} from "./trust";

// These tests are the B-EP09.3a acceptance: the universal Accept/Edit/Dismiss
// triad, "Edit flips a value to human-typed while retaining the original
// snippet" (§4.4), staged vs real as visibly distinct styles (§5c), and
// low confidence always shown (§4.2).

afterEach(cleanup);

// The components read copy from the i18n catalogs; assertions here use the
// English catalog so the strings under test are the spec's own wording.
const render = (ui: ReactNode) =>
  rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);

const proposal: Proposal = {
  description: "Set Brandt Automotive's deal value",
  value: "€48.000",
  agent: "capture",
  confidence: "med",
  evidence: { snippet: "…offer of 48k as discussed…", source: "email 12 Jun" },
};

describe("StagedProposal (B-EP09.3a)", () => {
  it("renders staged as visibly not-yet-real: staging style, agent provenance, confidence, evidence", () => {
    render(<StagedProposal proposal={proposal} />);
    const card = screen.getByRole("region", { name: "Proposed value" });
    expect(card.className).toContain("staging-card");
    expect(screen.getByText("Automated by capture")).toBeTruthy();
    expect(screen.getByText("medium")).toBeTruthy();
    expect(screen.getByText(/offer of 48k/)).toBeTruthy();
  });

  it("Accept persists the value with AGENT provenance kept", async () => {
    const onResolve = vi.fn();
    render(<StagedProposal proposal={proposal} onResolve={onResolve} />);
    await userEvent.click(screen.getByRole("button", { name: "Accept" }));

    expect(onResolve).toHaveBeenCalledWith({
      outcome: "accepted",
      value: "€48.000",
    });
    const card = screen.getByRole("region", { name: "Resolved value" });
    expect(card.className).toContain("real-card");
    expect(card.className).not.toContain("staging-card");
    expect(screen.getByText("Automated by capture")).toBeTruthy();
  });

  it("Edit flips the value to human-typed while RETAINING the original snippet (§4.4)", async () => {
    const onResolve = vi.fn();
    render(<StagedProposal proposal={proposal} onResolve={onResolve} />);
    await userEvent.click(screen.getByRole("button", { name: "Edit" }));

    const input = screen.getByRole("textbox", { name: /Edit Set Brandt/ });
    await userEvent.clear(input);
    await userEvent.type(input, "€45.000");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(onResolve).toHaveBeenCalledWith({
      outcome: "edited",
      value: "€45.000",
    });
    expect(screen.getByText("Typed by you")).toBeTruthy();
    expect(screen.queryByText("Automated by capture")).toBeNull();
    // the original evidence snippet is still attached to the edited value
    expect(screen.getByText(/offer of 48k/)).toBeTruthy();
  });

  it("Dismiss resolves without leaving a value behind", async () => {
    const onResolve = vi.fn();
    render(<StagedProposal proposal={proposal} onResolve={onResolve} />);
    await userEvent.click(screen.getByRole("button", { name: "Dismiss" }));

    expect(onResolve).toHaveBeenCalledWith({ outcome: "dismissed" });
    expect(screen.queryByText(/€48.000/)).toBeNull();
    expect(screen.getByText("Suggestion dismissed")).toBeTruthy();
  });
});

describe("ConfidenceMeter", () => {
  it("shows low as low — there is no way to hide it (§4.2)", () => {
    render(<ConfidenceMeter level="low" />);
    const meter = screen.getByText("low");
    expect(meter.className).toContain("confidence-low");
  });
});

// The badge a label renders in. Its tone is the provenance claim: indigo says a
// model wrote it, and every other arm is the neutral badge.
function badgeReading(text: string): HTMLElement {
  const badge = screen.getByText(text).closest<HTMLElement>(".badge");
  if (!badge) {
    throw new Error(`"${text}" rendered outside a badge`);
  }
  return badge;
}

function claimsAModel(text: string): boolean {
  return badgeReading(text).classList.contains("badge-ai");
}

describe("ProvenanceTag", () => {
  it("distinguishes agent-written from human-typed", () => {
    render(<ProvenanceTag provenance={{ kind: "agent", agent: "runner" }} />);
    render(<ProvenanceTag provenance={{ kind: "human", self: true }} />);
    expect(claimsAModel("Automated by runner")).toBe(true);
    expect(claimsAModel("Typed by you")).toBe(false);
  });

  // "Typed by you" over a colleague's entry is a false statement about who to
  // ask, and it used to be what an UNATTRIBUTED row said too — the two cases a
  // reader most needs kept apart both read as their own handiwork.
  it("names another contact rather than claiming the reader typed it", () => {
    render(
      <ProvenanceTag
        provenance={{ kind: "human", self: false, userId: "u-2" }}
        renderUser={(id) => <span>Christian ({id})</span>}
      />,
    );
    expect(badgeReading("Christian (u-2)").textContent).toBe(
      "Typed by Christian (u-2)",
    );
    expect(screen.queryByText("Typed by you")).toBeNull();
  });

  it("says a contact entered it when it cannot say which contact", () => {
    render(<ProvenanceTag provenance={{ kind: "human", self: false }} />);
    expect(claimsAModel("Typed by a person")).toBe(false);
  });

  it("reads an unrecorded source as unknown, not as the reader", () => {
    render(<ProvenanceTag provenance={{ kind: "unknown" }} />);
    expect(claimsAModel("Source not recorded")).toBe(false);
    expect(screen.queryByText("Typed by you")).toBeNull();
  });

  // A buyer and an unrecorded source are one branch apart, and collapsing the
  // first into the second is what put "Source not recorded" on a row whose
  // source was a contact. The two are asserted together because that is the
  // distinction: `unknown` still has to mean nobody recorded a source.
  it("reads a buyer as a contact from outside, never as an unrecorded source", () => {
    render(<ProvenanceTag provenance={{ kind: "buyer" }} />);
    render(<ProvenanceTag provenance={{ kind: "unknown" }} />);

    expect(claimsAModel("Typed by a buyer")).toBe(false);
    // Not the colleague arm either: a buyer holds no seat, and the colleague
    // arm's "Typed by a person" would send a reader to the member directory.
    expect(screen.queryByText("Typed by a person")).toBeNull();
    expect(claimsAModel("Source not recorded")).toBe(false);
  });

  // A connector copies what a mailbox already held; no model decided it, so
  // the badge names the connector without the AI tone.
  it("names the connector a record was imported through", () => {
    render(
      <ProvenanceTag provenance={{ kind: "connector", connector: "gmail" }} />,
    );
    expect(claimsAModel("Via gmail")).toBe(false);
  });

  // A background job and an AI agent are different answers to "who do I ask",
  // so they take different wording and different chrome. Drawn in the agent
  // tone, a scheduled sweep would tell a reader a model decided something.
  it("reads a job the installation ran as the system, not as an agent", () => {
    render(
      <ProvenanceTag provenance={{ kind: "system", job: "close-date" }} />,
    );
    expect(claimsAModel("System task close-date")).toBe(false);
  });

  // The two unnamed cases: an agent behind a passport uuid, and a job that
  // stamped no id. Both say the KIND — which is what the wire recorded —
  // instead of printing an identifier a reader can do nothing with.
  it("says the kind and stops when the actor has no name to print", () => {
    render(<ProvenanceTag provenance={{ kind: "agent" }} />);
    render(<ProvenanceTag provenance={{ kind: "system" }} />);
    expect(claimsAModel("Automated by an agent")).toBe(true);
    expect(claimsAModel("System task")).toBe(false);
  });

  // An import runs as ONE administrator, so captured_by names that seat on
  // every row it wrote and `self` is true for them across all of it. That is
  // how a migration comes to tell one colleague they typed a decade of
  // everybody else's correspondence. The author is the only field that knows
  // better, so it is read before `self` is.
  it("names the author of an imported row instead of the reader who imported it", () => {
    render(
      <ProvenanceTag
        provenance={{
          kind: "human",
          self: true,
          userId: "u-lars",
          author: { display_name: "Mutaz Suleiman", via: "HubSpot" },
        }}
      />,
    );
    expect(badgeReading("Logged in HubSpot by Mutaz Suleiman")).toBeTruthy();
    expect(screen.queryByText("Typed by you")).toBeNull();
  });

  // An author who never held a seat here has a name and nothing else — 36 of
  // the 76 colleagues a HubSpot import carries are that. The row still says
  // who wrote it; only the system it came from goes unnamed.
  it("names an author whose source system was not recorded", () => {
    render(
      <ProvenanceTag
        provenance={{
          kind: "human",
          self: true,
          author: { display_name: "Shayne Doherty" },
        }}
      />,
    );
    expect(badgeReading("Logged by Shayne Doherty")).toBeTruthy();
    expect(screen.queryByText("Typed by you")).toBeNull();
  });

  // The author outranks `renderUser` too. A colleague who still holds a seat
  // resolves through the member directory on every other row, but on an
  // imported one the directory would name whoever captured_by points at —
  // the administrator again, not the author.
  it("prefers the author over the seat the row was captured under", () => {
    render(
      <ProvenanceTag
        provenance={{
          kind: "human",
          self: false,
          userId: "u-lars",
          author: { display_name: "Mutaz Suleiman", via: "HubSpot" },
        }}
        renderUser={(id) => <span>Lars ({id})</span>}
      />,
    );
    expect(badgeReading("Logged in HubSpot by Mutaz Suleiman")).toBeTruthy();
    expect(screen.queryByText(/^Typed by Lars/)).toBeNull();
  });

  // No author is every other row in the product, which must read exactly as it
  // did before this branch existed.
  it("leaves a row with no author reading as it always did", () => {
    render(<ProvenanceTag provenance={{ kind: "human", self: true }} />);
    expect(claimsAModel("Typed by you")).toBe(false);
  });
});

// WHERE in the source a claim came from. A quoted sentence with no address is
// a claim the reader has to take on trust; a line reference is what lets them
// go back to the exchange and check it.
describe("the lines an evidence chip was read from", () => {
  it("closes a run into one range and keeps a gap apart", () => {
    expect(formatSourceLines([12, 13, 14])).toBe("12–14");
    expect(formatSourceLines([3, 9])).toBe("3, 9");
    expect(formatSourceLines([7])).toBe("7");
  });

  it("orders and de-duplicates what the server sent, rather than trusting it", () => {
    expect(formatSourceLines([14, 12, 13, 12])).toBe("12–14");
  });

  it("shows the reference beside the snippet, in the reader's language", () => {
    render(
      <EvidenceChip
        evidence={{
          snippet: "I'll send the revised quote on Monday.",
          source: "transcript",
          lines: [12, 13, 14],
        }}
      />,
    );
    expect(screen.getByText("lines 12–14")).toBeTruthy();
  });

  it("says line, singular, for a claim read from one line", () => {
    render(
      <EvidenceChip
        evidence={{
          snippet: "Monday it is.",
          source: "transcript",
          lines: [7],
        }}
      />,
    );
    expect(screen.getByText("line 7")).toBeTruthy();
  });

  it("adds nothing to a source that has no lines to point at", () => {
    render(
      <EvidenceChip
        evidence={{ snippet: "…offer of 48k…", source: "email 12 Jun" }}
      />,
    );
    expect(screen.queryByText(/^lines? /)).toBeNull();
  });
});

// A chip exists to make a claim checkable by a reader. A record reference —
// the shape the contract documents an evidence source to be — is checkable by
// the system and by nobody else, so the reader gets the record KIND and the row
// id stays in the title.
describe("the source an evidence chip names", () => {
  const LEAD_REF = "lead:019fff1e-8439-75fe-adfe-78ab4b497f12";

  it("names the record kind, never the row id, when the source is a record reference", () => {
    const { container } = render(
      <EvidenceChip
        evidence={{ snippet: "Ruebenase Gert", source: LEAD_REF }}
      />,
    );
    const chip = container.querySelector(".evidence-chip");
    expect(chip?.textContent).toBe('"Ruebenase Gert" · lead');
    expect(chip?.getAttribute("title")).toBe(LEAD_REF);
  });

  it("keeps the row id out of the compact form too, where the source is all there is", () => {
    const { container } = render(
      <EvidenceChip
        evidence={{ snippet: "Ruebenase Gert", source: LEAD_REF }}
        collapsed
      />,
    );
    const toggle = container.querySelector(".evidence-chip-toggle");
    expect(toggle?.textContent).toBe("lead");
    expect(toggle?.getAttribute("title")).toBe(LEAD_REF);
  });

  it("shows a source somebody wrote exactly as written, tooltip and all", () => {
    // A colon does not make a source a record reference: these two are words,
    // and shortening either would hide what the claim rests on.
    for (const source of ["email 12 Jun", "deal_coverage_risk:margin_thin"]) {
      cleanup();
      const { container } = render(
        <EvidenceChip evidence={{ snippet: "…offer of 48k…", source }} />,
      );
      const chip = container.querySelector(".evidence-chip");
      expect(chip?.textContent).toBe(`"…offer of 48k…" · ${source}`);
      expect(chip?.hasAttribute("title")).toBe(false);
    }
  });
});

// toEvidence is the boundary the untyped contract value crosses, so it owes the
// narrowing to every caller rather than trusting one: a screen that has to
// assert a shape before handing it over has done the checking itself, unchecked.
describe("narrowing a contract value to evidence", () => {
  it("accepts an object carrying both fields as strings", () => {
    expect(
      toEvidence({ snippet: "…48k as discussed…", source: "email" }),
    ).toEqual({ snippet: "…48k as discussed…", source: "email" });
  });

  it("keeps only the trust vocabulary's two fields", () => {
    expect(
      toEvidence({ snippet: "s", source: "email", confidence: 0.9 }),
    ).toEqual({ snippet: "s", source: "email" });
  });

  it("reads anything else as no evidence rather than guessing one", () => {
    // A missing field, a field of the wrong type, and the values a free-form
    // contract field can carry that are no object with those two fields at all.
    for (const raw of [
      { snippet: "s" },
      { source: "email" },
      { snippet: 12, source: "email" },
      "email 12 Jun",
      42,
      null,
      undefined,
      ["snippet", "source"],
    ]) {
      expect(toEvidence(raw)).toBeNull();
    }
  });
});

describe("confidenceLevel", () => {
  it("maps numeric confidence onto the three-glyph vocabulary", () => {
    expect(confidenceLevel(0.9)).toBe("high");
    expect(confidenceLevel(0.6)).toBe("med");
    expect(confidenceLevel(0.2)).toBe("low");
  });

  it("bands on the boundary values themselves", () => {
    // The thresholds are inclusive, and eight surfaces read them: a 0.8 that
    // banded "med" here and "high" on the next screen is the drift this one
    // home exists to prevent.
    expect(confidenceLevel(0.8)).toBe("high");
    expect(confidenceLevel(0.5)).toBe("med");
    expect(confidenceLevel(0.499)).toBe("low");
  });

  it("reads an unrecorded confidence as no reading, never as a low one", () => {
    expect(confidenceLevel(null)).toBeNull();
    expect(confidenceLevel(undefined)).toBeNull();
  });
});
