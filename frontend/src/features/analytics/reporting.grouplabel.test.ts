import { describe, expect, it } from "vitest";
import { type Translator, translate } from "../../i18n";
import { breakdownLabel } from "./reporting.grouplabel";

const german: Translator = (key, params) => translate("de", key, params);

describe("a breakdown group's name", () => {
  it("reads a fixed group in the reader's language, keyed by its group key", () => {
    expect(breakdownLabel("status:engaged", "Engaged", german)).toBe(
      "Im Gespräch",
    );
    expect(breakdownLabel("origin:", "No origin", german)).toBe(
      "Keine Herkunft",
    );
    expect(breakdownLabel("activity:inactive", "Inactive", german)).toBe(
      "Inaktiv",
    );
    expect(
      breakdownLabel("partner:hidden", "Partner you cannot see", german),
    ).toBe("Nicht sichtbares Partnerunternehmen");
  });

  it("keeps a name read from a record, and reads only the partner's role", () => {
    expect(breakdownLabel("origin:web", "Website", german)).toBe("Website");
    expect(
      breakdownLabel(
        "partner:p-1:influenced",
        "Nord · Süd · Influenced",
        german,
      ),
    ).toBe("Nord · Süd · Beeinflusst");
    expect(breakdownLabel("partner:p-1", "Nordwind", german)).toBe("Nordwind");
  });
});
