/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import type { CompanyFieldName } from "../onboarding";
import { type DeckCard, ReviewDeck } from "./review-deck";

// The tray's two numbers, which are two different questions: how many cards
// are still outstanding, and how many the reader is being walked through.
//
// Both were `cards.length` once, so the tray read "N of N left" for the whole
// deck and never moved. Reaching for the queue instead exposed the other half:
// the deck opens on the proposal and the site read prefills the draft on a
// query that lands afterwards, so for one commit nearly every field is
// outstanding, and a queue that never shortened kept all of them as cards to
// walk. This file pins the pair — the count falls as answers land, and the
// queue drops what the READ settled, without renumbering the card in view.

afterEach(cleanup);

const LABELS: Readonly<Record<string, string>> = {
  display_name: "Company name",
  icp: "Ideal customer",
  offer_summary: "What do you sell?",
};
const FIELDS = Object.keys(LABELS) as CompanyFieldName[];

/**
 * The deck under the parent's own arrangement: `cards` is what is still
 * outstanding, `cardOf` answers for every field, and the value the reader types
 * is read live. `prefilled` is the site read landing — handed in by the test so
 * it can arrive after the deck has opened, which is the whole case.
 */
function Deck({
  prefilled = {},
}: Readonly<{ prefilled?: Readonly<Partial<Record<string, string>>> }>) {
  const [typed, setTyped] = useState<Readonly<Record<string, string>>>({});
  const cardFor = (field: CompanyFieldName): DeckCard => ({
    field,
    question: LABELS[field] ?? field,
    required: true,
    multiline: false,
    value: typed[field] ?? prefilled[field] ?? "",
  });
  const all = FIELDS.map(cardFor);
  return (
    <LocaleProvider>
      <ReviewDeck
        cards={all.filter((entry) => entry.value === "")}
        cardOf={cardFor}
        settled={0}
        onField={(field, value) =>
          setTyped((current) => ({ ...current, [field]: value }))
        }
        onDone={() => {}}
        pending={false}
        blockers={[]}
        held={false}
        openQuestions={0}
        digest={() => null}
      />
    </LocaleProvider>
  );
}

it("counts down as answers land, against the queue rather than itself", async () => {
  const user = userEvent.setup();
  render(<Deck />);

  expect(screen.getByText("3 of 3 left")).toBeInTheDocument();

  // One character is enough: it drops `display_name` off the outstanding list
  // while leaving it the card in view, which is exactly the moment the tray has
  // to say something other than "N of N".
  await user.type(screen.getByRole("textbox", { name: "Company name" }), "G");

  expect(screen.getByText("2 of 3 left")).toBeInTheDocument();
});

it("drops a field the read settled ahead of the cursor, and keeps one behind it", async () => {
  const user = userEvent.setup();
  const { rerender } = render(<Deck />);
  expect(screen.getByText("3 of 3 left")).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Next" }));
  expect(screen.getByText("Ideal customer")).toBeInTheDocument();

  // BEHIND the cursor: the reader has already been shown this card, so the read
  // answering it now must not renumber the one in front of them. The
  // outstanding count falls; the queue does not.
  rerender(<Deck prefilled={{ display_name: "Gradion" }} />);
  expect(screen.getByText("2 of 3 left")).toBeInTheDocument();

  // AHEAD of it: a card the reader has not reached and the read has settled was
  // never their question, so it leaves the queue entirely.
  rerender(
    <Deck prefilled={{ display_name: "Gradion", offer_summary: "Tools" }} />,
  );
  expect(screen.getByText("1 of 2 left")).toBeInTheDocument();
});
