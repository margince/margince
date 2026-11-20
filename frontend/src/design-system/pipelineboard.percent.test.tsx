/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { formatPercent } from "../format/format";
import { type Locale, LocaleProvider } from "../i18n";
import {
  type BoardDeal,
  type BoardMoneyColumn,
  PipelineBoard,
} from "./pipelineboard";

afterEach(() => {
  cleanup();
});

const deal: BoardDeal = {
  id: "d-1",
  name: "Contoso renewal",
  company: "Acme GmbH",
  companyId: "o-1",
  valueMinor: 12_000,
  currency: "EUR",
  probabilityPct: 10,
  idleSince: "2026-09-12T12:00:00Z",
  measuredAt: Date.parse("2026-09-15T12:00:00Z"),
};

const qualified: BoardMoneyColumn = {
  stage: "qualified",
  label: "Qualified",
  probabilityPct: 10,
  rawMinor: 12_000,
  weightedMinor: 1_200,
  currency: "EUR",
  deals: [deal],
};

function renderBoard(locale: Locale) {
  render(
    <LocaleProvider initial={locale}>
      <PipelineBoard
        columns={[qualified]}
        cardHref={(one) => `#/deals/${one.id}`}
        zone="Europe/Berlin"
      />
    </LocaleProvider>,
  );
}

// The stage header and the card write one figure, so they write it one way:
// in the locale's own percent form, which in German sets a no-break space.
describe("the board's percent", () => {
  it.each<[Locale, string]>([
    ["de", "10 %"],
    ["en", "10%"],
  ])(
    "reads the same in the header and on the card in %s",
    async (locale, written) => {
      renderBoard(locale);
      expect(formatPercent(0.1, locale)).toBe(written);
      // Read raw: the default text matcher folds the no-break space into a
      // plain one, and the space is the subject.
      const header = await screen.findByText("Qualified");
      const column = header.closest("section");
      expect(column?.querySelector(".prob")?.textContent).toBe(written);
      expect(column?.querySelector(".probability-ring")?.textContent).toContain(
        written,
      );
    },
  );
});
