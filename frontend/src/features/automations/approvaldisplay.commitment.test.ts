// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { type Locale, translate } from "../../i18n";
import { resolveDisplay } from "./approvaldisplay";

// The server writes a commitment's provenance sentence once, in its own
// language. The card shows it once, marked as the original text, never passed
// off as a translation or a draft.

const day = (value: string) => value.split("-").reverse().join(".");

function card(
  locale: Locale,
  change: Record<string, unknown>,
): readonly (readonly [string, string | null])[] {
  const t = (
    key: Parameters<typeof translate>[1],
    params?: Record<string, string>,
  ) => translate(locale, key, params);
  return resolveDisplay("commitment_task", change, t, day).map(
    (row) => [row.label, row.value] as const,
  );
}

const BODY =
  'Promiser: Lena Fischer. Commitment in the email "Contract", sent 2026-10-01.';

const mailProposal = {
  summary: "Send the signed form by Friday",
  party: "Lena Fischer",
  body: BODY,
  quote: "I will send the signed form by Friday",
  locator: "abc",
  links: [],
};

describe("a commitment proposal's card", () => {
  it.each([
    ["de", "Originaltext"],
    ["vi", "Văn bản gốc"],
  ] as const)(
    "labels the server's source text as the original (%s)",
    (locale, label) => {
      const rows = card(locale, mailProposal);
      expect(rows).toContainEqual([label, BODY]);
      expect(rows.filter(([, value]) => value === BODY)).toHaveLength(1);
    },
  );
});
