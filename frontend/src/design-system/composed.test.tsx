/** @vitest-environment jsdom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { MONEY_ABSENT } from "../format/format";
import { LocaleProvider } from "../i18n";
import {
  type BoardColumn,
  type BoardMoneyColumn,
  DealCard,
  PipelineBoard,
  RecordView,
} from "./composed";

// B-EP09.3b acceptance: the composed surfaces consume the 3a primitives and
// the staged / real / human-typed three-way distinction carries through.

/** What `FieldGuard mode="masked"` announces itself as (`rbac.masked`). */
const MASK = "Masked value";

afterEach(cleanup);

const render = (ui: ReactNode) =>
  rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);

describe("DealCard + PipelineBoard", () => {
  const deal = {
    id: "d1",
    name: "Fleet retrofit",
    org: "Brandt Automotive",
    valueMinor: 4_800_000,
    currency: "EUR",
    ageMs: 62 * 86_400_000,
    stalled: true,
  };

  // Staleness reaches the reader as a WORD, and only as a word: a card that also
  // carried an edge stripe said one thing twice, and the half of it that a
  // reader who cannot see colour gets is the badge.
  it("renders value/age and the stalled aging flag (AC-pipeline-5)", () => {
    render(<DealCard deal={deal} href="#/deals/d1" zone="Europe/Berlin" />);
    expect(screen.getByText("€48,000.00")).toBeTruthy();
    expect(screen.getByText("stalled")).toBeTruthy();
    expect(screen.getByRole("link").className).not.toContain("stalled");
  });

  // A company has three readings on a card and only one of them is blank. The
  // named one carries its monogram, the withheld one carries the mask every
  // other surface draws over a withheld value, and a deal that names no company
  // draws nothing — the one case where an empty slot states the truth.
  it("names a company it was given, with the monogram beside it", () => {
    render(<DealCard deal={deal} href="#/deals/d1" zone="Europe/Berlin" />);
    expect(screen.getByText("Brandt Automotive")).toBeTruthy();
    expect(screen.getByText("BA")).toBeTruthy();
    expect(screen.queryByLabelText(MASK)).toBeNull();
  });

  it("draws the mask over a withheld company, never words for it", () => {
    const { container } = render(
      <DealCard
        deal={{ ...deal, org: "", orgWithheld: true }}
        href="#/deals/d1"
        zone="Europe/Berlin"
      />,
    );
    expect(screen.getByLabelText(MASK)).toBeTruthy();
    // No name and no mark beside it: a monogram cut from the word for
    // "withheld" would be a mark no company has.
    expect(container.querySelector(".deal-org-name")).toBeNull();
    expect(container.querySelector(".avatar")).toBeNull();
  });

  // The owner is a mark at the head line's edge, and the mark carries the NAME:
  // a monogram a teammate has to decode is not an answer, so the full name is
  // its label. A deal nobody owns — or whose owner the caller could not name —
  // draws no slot rather than a blank one.
  it("marks the owner it was given, labelled by name, and no slot for a deal without one", () => {
    const { container, rerender } = render(
      <DealCard
        deal={{ ...deal, owner: "Ada Lindqvist" }}
        href="#/deals/d1"
        zone="Europe/Berlin"
      />,
    );
    expect(screen.getByLabelText("Ada Lindqvist")).toBeTruthy();
    rerender(
      <DealCard
        deal={{ ...deal, owner: null }}
        href="#/deals/d1"
        zone="Europe/Berlin"
      />,
    );
    expect(container.querySelector(".deal-owner")).toBeNull();
  });

  it("draws no company slot at all for a deal that names none", () => {
    const { container } = render(
      <DealCard
        deal={{ ...deal, org: "" }}
        href="#/deals/d1"
        zone="Europe/Berlin"
      />,
    );
    expect(screen.queryByLabelText(MASK)).toBeNull();
    expect(container.querySelector(".deal-org")).toBeNull();
  });

  it("a staged deal renders visibly distinct from a real one", () => {
    const { container } = render(
      <>
        <DealCard
          deal={{ ...deal, id: "real", stalled: false }}
          href="#/deals/real"
          zone="Europe/Berlin"
        />
        <DealCard
          deal={{ ...deal, id: "staged", stalled: false, staged: true }}
          href="#/deals/staged"
          zone="Europe/Berlin"
        />
      </>,
    );
    const [real, staged] = Array.from(container.querySelectorAll(".deal-card"));
    expect(real.className).not.toContain("staged");
    expect(staged.className).toContain("staged");
  });

  it("board columns render probability, count, raw and weighted sub-lines", () => {
    const column: BoardMoneyColumn = {
      stage: "proposal",
      label: "Proposal",
      probabilityPct: 40,
      rawMinor: 6_050_000,
      weightedMinor: 2_420_000,
      currency: "EUR",
      deals: [deal],
    };
    render(
      <PipelineBoard
        columns={[column]}
        cardHref={(d) => `#/deals/${d.id}`}
        zone="Europe/Berlin"
      />,
    );
    expect(screen.getByText("40%")).toBeTruthy();
    expect(screen.getByText("1 deals")).toBeTruthy();
    expect(screen.getByText("€60,500.00")).toBeTruthy();
    expect(screen.getByText("weighted €24,200.00")).toBeTruthy();
  });

  // A caller with a capped card fetch (the Kanban board) supplies
  // the stage's TRUE count from a server aggregate — it must render that,
  // not deals.length, whenever the two disagree.
  it("renders the column's own count over deals.length when the two disagree", () => {
    const column: BoardMoneyColumn = {
      stage: "proposal",
      label: "Proposal",
      probabilityPct: 40,
      rawMinor: 6_050_000,
      weightedMinor: 2_420_000,
      currency: "EUR",
      deals: [deal],
      count: 137,
    };
    render(
      <PipelineBoard
        columns={[column]}
        cardHref={(d) => `#/deals/${d.id}`}
        zone="Europe/Berlin"
      />,
    );
    expect(screen.getByText("137 deals")).toBeTruthy();
    expect(screen.queryByText("1 deals")).toBeNull();
  });

  // A refused sum says it was refused. Dropping the figure and leaving the count
  // alone is a blank where a total belongs, and a blank cannot tell "these are in
  // several currencies, so no one total means anything" apart from "nobody has
  // priced these" — which reads as a column that failed to load. On a board whose
  // stages hold euros, dollars and dong, most columns are in this state.
  it("says why a mixed-currency column shows no total", () => {
    const column: BoardMoneyColumn = {
      stage: "won",
      label: "Won",
      probabilityPct: 100,
      rawMinor: null,
      weightedMinor: null,
      currency: null,
      deals: [deal],
      count: 29,
      sumHidden: true,
    };
    render(
      <PipelineBoard
        columns={[column]}
        cardHref={(d) => `#/deals/${d.id}`}
        zone="Europe/Berlin"
      />,
    );
    expect(
      screen.getByText("several currencies — no single total"),
    ).toBeTruthy();
    // The count is a fact and stays; no money figure is invented beside it.
    expect(screen.getByText("29 deals")).toBeTruthy();
    expect(screen.queryByText(/weighted/)).toBeNull();
  });

  // A money figure is an amount AND its currency. Either half absent leaves no
  // figure to draw, and both substitutes state something false: a zero is an
  // amount the server never sent, and a currency sign the card chose cannot be
  // told apart from one the deal actually carries.
  it("a deal with an amount but no currency states no figure, never a currency the card chose", () => {
    render(
      <DealCard
        deal={{ ...deal, currency: null }}
        href="#/deals/d1"
        zone="Europe/Berlin"
      />,
    );
    expect(screen.getByText(MONEY_ABSENT)).toBeTruthy();
    expect(screen.queryByText(/48,000/)).toBeNull();
  });

  it("a deal with a currency but no amount states no figure, never a zero", () => {
    render(
      <DealCard
        deal={{ ...deal, valueMinor: null }}
        href="#/deals/d1"
        zone="Europe/Berlin"
      />,
    );
    expect(screen.getByText(MONEY_ABSENT)).toBeTruthy();
    expect(screen.queryByText("€0.00")).toBeNull();
  });

  // The count is a fact the column has even when the total is not: a stage of
  // deals nobody priced still holds them, and a zero total beside that count
  // reads as an empty stage.
  it("a column whose total names no currency draws both figures as absent and keeps its count", () => {
    const column: BoardMoneyColumn = {
      stage: "proposal",
      label: "Proposal",
      probabilityPct: 40,
      rawMinor: null,
      weightedMinor: null,
      currency: null,
      deals: [deal],
      count: 4,
    };
    render(
      <PipelineBoard
        columns={[column]}
        cardHref={(d) => `#/deals/${d.id}`}
        zone="Europe/Berlin"
      />,
    );
    expect(screen.getByText("4 deals")).toBeTruthy();
    expect(screen.getByText(`weighted ${MONEY_ABSENT}`)).toBeTruthy();
    expect(
      screen.getByText(MONEY_ABSENT, { selector: ".board-col-money" }),
    ).toBeTruthy();
    expect(screen.queryByText(/€0\.00/)).toBeNull();
  });

  it("lets a non-deal board provide its own record noun", () => {
    const column: BoardColumn<{ id: string; name: string }> = {
      stage: "new",
      label: "New",
      deals: [{ id: "lead-1", name: "Ada" }],
    };
    render(
      <PipelineBoard
        variant="plain"
        columns={[column]}
        countLabel={(count) => `${count} leads`}
        renderCard={(card) => <span>{card.name}</span>}
      />,
    );
    expect(screen.getByText("1 leads")).toBeTruthy();
    expect(screen.queryByText("1 deals")).toBeNull();
  });
});

describe("RecordView + timeline", () => {
  it("renders the header and provenance-tagged timeline in the workspace zone", () => {
    render(
      <RecordView
        name="Anna Weber"
        subtitle="Head of Procurement · Brandt Automotive"
        zone="Europe/Berlin"
        timeline={[
          {
            id: "t1",
            kind: "email",
            title: "Re: fleet retrofit offer",
            atIso: "2026-06-12T09:00:00Z",
            provenance: { kind: "agent", agent: "capture" },
          },
          {
            id: "t2",
            kind: "note",
            title: "Call notes",
            atIso: "2026-06-14T10:00:00Z",
            provenance: { kind: "human", self: true },
          },
        ]}
      />,
    );
    expect(
      screen.getByRole("heading", { level: 1, name: "Anna Weber" }),
    ).toBeTruthy();
    expect(screen.getByText("12/06/2026")).toBeTruthy();
    expect(screen.getByText("Automated by capture")).toBeTruthy();
    expect(screen.getByText("typed by you")).toBeTruthy();
  });

  it("keeps the whole message in the document, clamped but never cut", () => {
    // The clamp is CSS. Truncating the string instead would put the rest of
    // the message out of reach of find-in-page, selection and a screen reader,
    // and no toggle can give back text that was never rendered.
    const body = `Moin Christian, ${"eine sehr lange Zeile ".repeat(20)}Ende.`;
    render(
      <RecordView
        name="ScaleCommerce"
        zone="Europe/Berlin"
        timeline={[
          {
            id: "t1",
            kind: "email",
            title: "Update zu Margince",
            atIso: "2026-07-17T09:00:00Z",
            provenance: { kind: "agent", agent: "capture" },
            body,
          },
        ]}
      />,
    );
    expect(screen.getByText(body.trim())).toBeTruthy();
  });

  it("says nothing where a message was lawfully erased", () => {
    // Retention and Art. 17 both null the body. A row whose message is gone
    // must render as a row with no message, not as an empty quote.
    render(
      <RecordView
        name="ScaleCommerce"
        zone="Europe/Berlin"
        timeline={[
          {
            id: "t1",
            kind: "email",
            title: "Update zu Margince",
            atIso: "2026-07-17T09:00:00Z",
            provenance: { kind: "agent", agent: "capture" },
            body: "   ",
          },
        ]}
      />,
    );
    expect(document.querySelector(".tl-text")).toBeNull();
  });

  it("renders a timeline entry's action slot when present", () => {
    render(
      <RecordView
        name="Acme"
        zone="UTC"
        timeline={[
          {
            id: "a1",
            kind: "email",
            title: "Re: Q3",
            atIso: "2026-07-01T00:00:00Z",
            provenance: { kind: "human", self: true },
            actions: (
              <button type="button" key="reply">
                Reply
              </button>
            ),
          },
        ]}
      />,
    );
    expect(screen.getByRole("button", { name: "Reply" })).toBeTruthy();
  });
});

describe("TimelineText on a mail row", () => {
  const SIGNED =
    "From: anna@kunde.de\nTo: lars@gradion.com\n\nKönnen wir Dienstag über das Angebot sprechen?\n\nMit freundlichen Grüßen\nAnna Berger\nKunde GmbH";

  const mailRow = (body: string, kind: "email" | "note" = "email") => [
    {
      id: "a1",
      kind,
      title: "Re: Angebot",
      atIso: "2026-07-01T00:00:00Z",
      provenance: { kind: "human" as const, self: true },
      body,
    },
  ];

  // The email branch replaced ONE kind's reading. These are the other six —
  // `change` included, because a field edit rendered through a message's row
  // would be the same collapse this component made once by drawing a call as a
  // note. Each carries an emailSummary it must ignore, so the test proves the
  // `kind === "email"` half of the guard rather than the half that is true by
  // accident.
  const IGNORED_SUMMARY = {
    activity_id: "a-ignored",
    occurred_at: "2026-07-01T00:00:00Z",
    display_status: "team" as const,
    attachment_count: 0,
    move: "none" as const,
    version: 1,
    subject: "Ein Betreff, der nicht gezeichnet werden darf",
  };

  it("leaves every kind that is not an email drawing its own body", () => {
    for (const kind of [
      "note",
      "call",
      "meeting",
      "task",
      "message",
      "change",
    ] as const) {
      const { unmount } = render(
        <RecordView
          name="Acme"
          zone="UTC"
          timeline={[
            {
              id: `a-${kind}`,
              kind,
              title: "Was besprochen wurde",
              atIso: "2026-07-01T00:00:00Z",
              provenance: { kind: "human" as const, self: true },
              body: "Der ganze Text, den dieser Eintrag traegt.",
              emailSummary: IGNORED_SUMMARY,
            },
          ]}
        />,
      );
      expect(
        screen.getByText("Der ganze Text, den dieser Eintrag traegt."),
      ).toBeTruthy();
      // The summary is there and unused: only an email draws through it.
      expect(
        screen.queryByText("Ein Betreff, der nicht gezeichnet werden darf"),
      ).toBeNull();
      unmount();
    }
  });

  // The canonical row, drawn: an email WITH its summary goes through
  // EmailEntry, which is the whole point of the branch.
  it("draws an email with its summary through the canonical row", () => {
    render(
      <RecordView
        name="Acme"
        zone="UTC"
        timeline={[
          {
            id: "a-email",
            kind: "email",
            title: "Re: Angebot",
            atIso: "2026-07-01T09:12:00Z",
            provenance: { kind: "human" as const, self: true },
            body: "Der Text der Mail.",
            emailSummary: {
              ...IGNORED_SUMMARY,
              activity_id: "a-email",
              subject: "Re: Angebot",
              preview: "Können wir Dienstag sprechen?",
              direction: "inbound" as const,
              counterparty: "Anna Berger",
              move: "needs_reply" as const,
            },
          },
        ]}
      />,
    );
    // EmailEntry's own marks, which the generic row does not draw.
    expect(screen.getByText("Needs reply")).toBeTruthy();
    expect(screen.getByText("Können wir Dienstag sprechen?")).toBeTruthy();
  });

  // A withheld email whose server has not caught up carries no summary and
  // falls to the generic row. That row masked the title and went on printing
  // the counterparty above it — "Received from Anna" beside a message whose
  // subject it just refused still says who this record is talking to.
  it("names nobody on a withheld row, summary or not", () => {
    render(
      <RecordView
        name="Acme"
        zone="UTC"
        timeline={[
          {
            id: "a-withheld",
            kind: "email",
            title: "Re: Angebot",
            atIso: "2026-07-01T00:00:00Z",
            provenance: { kind: "human" as const, self: true },
            direction: "inbound",
            counterparts: "Anna Berger",
            withheld: true,
          },
        ]}
      />,
    );
    expect(screen.queryByText(/Anna Berger/)).toBeNull();
    expect(screen.queryByText("Re: Angebot")).toBeNull();
  });

  it("shows the message and hides the signature until asked", async () => {
    const user = userEvent.setup();
    render(<RecordView name="Acme" zone="UTC" timeline={mailRow(SIGNED)} />);

    expect(screen.getByText(/Können wir Dienstag/)).toBeTruthy();
    expect(screen.queryByText(/Mit freundlichen Grüßen/)).toBeNull();

    await user.click(
      screen.getByRole("button", { name: "Show signature and quoted text" }),
    );
    expect(screen.getByText(/Mit freundlichen Grüßen/)).toBeTruthy();
    expect(screen.getByText(/Kunde GmbH/)).toBeTruthy();
  });

  it("folds the signature away again", async () => {
    const user = userEvent.setup();
    render(<RecordView name="Acme" zone="UTC" timeline={mailRow(SIGNED)} />);

    await user.click(
      screen.getByRole("button", { name: "Show signature and quoted text" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Hide signature and quoted text" }),
    );
    expect(screen.queryByText(/Mit freundlichen Grüßen/)).toBeNull();
  });

  it("keeps the correspondents' addresses above the message", () => {
    // The preamble says who wrote to whom, which is part of reading a mail on
    // a record. It is the row TITLE that must not lead with it — see the
    // timelineTitle rule in people.tsx — not the message body.
    render(<RecordView name="Acme" zone="UTC" timeline={mailRow(SIGNED)} />);
    const body = document.querySelector(".tl-text-clamp")?.textContent ?? "";
    expect(body).toContain("anna@kunde.de");
    expect(body).toContain("Können wir Dienstag");
  });

  it("leaves a note whose text reads like a sign-off intact", () => {
    render(
      <RecordView
        name="Acme"
        zone="UTC"
        timeline={mailRow("Viele Grüße an das Team ausgerichtet.", "note")}
      />,
    );
    expect(screen.getByText(/Viele Grüße an das Team/)).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: "Show signature and quoted text",
      }),
    ).toBeNull();
  });

  it("offers no reveal when a mail carries no signature or quote", () => {
    render(
      <RecordView
        name="Acme"
        zone="UTC"
        timeline={mailRow("Kurz: ja, Dienstag passt uns gut.")}
      />,
    );
    expect(
      screen.queryByRole("button", {
        name: "Show signature and quoted text",
      }),
    ).toBeNull();
  });

  it("renders a link with its address as the label", () => {
    render(
      <RecordView
        name="Acme"
        zone="UTC"
        timeline={mailRow(
          "Die Unterlagen liegen unter https://kunde.de/angebot bereit.",
        )}
      />,
    );
    const link = screen.getByRole("link", {
      name: "https://kunde.de/angebot",
    });
    expect(link.getAttribute("href")).toBe("https://kunde.de/angebot");
    expect(link.getAttribute("rel")).toContain("noopener");
  });

  it("folds the signature again when the row is given a different mail", async () => {
    // The row is keyed by activity id, so the component stays mounted when the
    // entry it renders is replaced. A reveal must not carry over to a mail the
    // reader never opened.
    const user = userEvent.setup();
    const { rerender } = rtlRender(
      <LocaleProvider initial="en">
        <RecordView name="Acme" zone="UTC" timeline={mailRow(SIGNED)} />
      </LocaleProvider>,
    );
    await user.click(
      screen.getByRole("button", { name: "Show signature and quoted text" }),
    );
    expect(screen.getByText(/Kunde GmbH/)).toBeTruthy();

    rerender(
      <LocaleProvider initial="en">
        <RecordView
          name="Acme"
          zone="UTC"
          timeline={mailRow("Neue Nachricht.\n\n-- \nMax Muster\nAndere GmbH")}
        />
      </LocaleProvider>,
    );
    expect(screen.queryByText(/Andere GmbH/)).toBeNull();
  });

  it("keeps a link inside the folded signature reachable once revealed", async () => {
    const user = userEvent.setup();
    render(
      <RecordView
        name="Acme"
        zone="UTC"
        timeline={mailRow(
          "Kurz: ja.\n\n-- \nAnna Berger\nhttps://kunde.de/impressum",
        )}
      />,
    );
    expect(screen.queryByRole("link")).toBeNull();
    await user.click(
      screen.getByRole("button", { name: "Show signature and quoted text" }),
    );
    expect(
      screen.getByRole("link", { name: "https://kunde.de/impressum" }),
    ).toBeTruthy();
  });
});
