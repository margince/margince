/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { ContactHowItStands } from "./contacthowitstands";

type Contact360 = components["schemas"]["Contact360"];

const AS_OF = "2026-08-13T09:00:00Z";
const REPLIED = "They replied after 30 quiet days.";
const COOLED = "The relationship cooled from strong to moderate.";

// A band-only reading (nothing_needed) over a moderate band, with changes that
// point opposite ways. The verdict sentence and the ring's caption must come
// from the same change, so the card never says both.
const VIEW: Contact360 = {
  as_of: AS_OF,
  contact: {
    id: "p-1",
    full_name: "Dana Buyer",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  },
  sections_omitted: [],
  moment: {
    claim_key: "moment:nothing_needed",
    evidence_fingerprint: "f",
    rule: "nothing_needed",
    headline: "Headline",
    why_now: "Why nothing_needed",
    confidence: "observed_fact",
    evidence: [],
    recommended_action: {
      kind: "log_activity",
      label: "Log",
      state: "available",
    },
  },
  strength: {
    score: 54,
    bucket: "moderate",
    factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
    inbound_90d: 4,
    outbound_90d: 6,
  },
};

function standingText(view: Contact360): string {
  const { container } = render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <ContactHowItStands view={view} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const parts = container.querySelectorAll(
    ".standing-sheet-sentence, .pe-standing-caption",
  );
  return Array.from(parts, (part) => part.textContent ?? "").join("\n");
}

afterEach(cleanup);

describe("ContactHowItStands", () => {
  it.each<[string, Contact360["relationship_changes"], string, string]>([
    [
      "the reply is newest",
      [
        {
          kind: "cooled",
          at: "2026-08-01T09:00:00Z",
          from_bucket: "strong",
          to_bucket: "moderate",
        },
        { kind: "replied_after_gap", at: "2026-08-10T09:00:00Z", days: 30 },
      ],
      REPLIED,
      COOLED,
    ],
    [
      "the cooling is newest",
      [
        { kind: "replied_after_gap", at: "2026-08-01T09:00:00Z", days: 30 },
        {
          kind: "cooled",
          at: "2026-08-10T09:00:00Z",
          from_bucket: "strong",
          to_bucket: "moderate",
        },
      ],
      COOLED,
      REPLIED,
    ],
  ])(
    "never pairs opposite changes on a band-only reading when %s",
    (_case, changes, shown, contrary) => {
      const text = standingText({ ...VIEW, relationship_changes: changes });
      expect(text).toContain(shown);
      expect(text).not.toContain(contrary);
    },
  );
});
