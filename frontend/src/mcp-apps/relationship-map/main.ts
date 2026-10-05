// The relationship map view: who_knows's colleagues, warmest first, drawn as
// the contact's own Routes panel draws them — a position, the colleague's
// chip, the interactions the warmth rests on, and the three-bar meter.
//
// WHY THE BAND AND THE COUNT TOGETHER. A strength score alone is the mystery
// number again, and the seam is careful about a specific case this view has to
// keep honest: strength is ABSENT when the band is "none", because never having
// spoken is not a score of zero. Rendering a missing strength as 0 would tell a
// rep a relationship decayed when none ever existed, so an absent score renders
// as absent.
//
// It renders and nothing else. Introducing someone is a human act with its own
// route; a button here would be this view inventing authority it was not given.

import type { StrengthBand } from "../../design-system/strengthmeter";
import { count, el, onResult, warned } from "../bridge";
import { avatar, panel, panelBody, strengthMeter } from "../parts";
import {
  asFiniteNumber,
  asList,
  asRecord,
  asText,
  type Warning,
} from "../types";
import "../view.css";

/**
 * The seam's bands, read as the app's. who_knows answers high / medium / low /
 * none, and the product says strong / moderate / weak / no contact everywhere a
 * reader meets a relationship, so the view speaks the product's words over the
 * seam's values. A band outside the set renders as its own word with no meter,
 * because the vocabulary belongs to the seam and a view that refused an unknown
 * value would go blank the first time one was added.
 */
const BANDS: ReadonlyMap<string, readonly [StrengthBand, string]> = new Map([
  ["high", ["strong", "strong"]],
  ["medium", ["moderate", "moderate"]],
  ["low", ["weak", "weak"]],
  ["none", ["none", "no contact"]],
]);

/** The envelope's code for "this read stopped at its bound". A bounded ranking
 *  is not the whole network, and the tool's contract is explicit that a model —
 *  or a view — told nothing will report it as one. */
const SWEEP_TRUNCATED = "sweep_truncated";

type Colleague = {
  name: string;
  userID: string;
  bucket: string;
  strength: number | null;
  interactions: number | null;
};

/**
 * known narrows the untrusted payload, filtering BEFORE anything is counted:
 * the head and the empty state both have to describe what will actually be
 * shown. Counting the raw list and rendering the filtered one is how a view says
 * "3 colleagues" above no rows.
 */
function known(data: Record<string, unknown>): Colleague[] {
  return asList(data.colleagues)
    .filter(
      (c): c is Record<string, unknown> => typeof c === "object" && c !== null,
    )
    .map((c) => ({
      name: asText(c.display_name) || asText(c.user_id),
      userID: asText(c.user_id),
      bucket: asText(c.strength_bucket),
      strength: asFiniteNumber(c.strength),
      interactions: asFiniteNumber(c.interactions_90d),
    }));
}

function interactionsText(interactions: number | null): string {
  if (interactions === 0) return "no interactions in 90 days";
  if (interactions === 1) return "1 interaction in 90 days";
  return `${count(interactions)} interactions in 90 days`;
}

function evidence(colleague: Colleague): string {
  const said = interactionsText(colleague.interactions);
  // Absent, not zero. See the note at the top of this file.
  return colleague.strength === null
    ? said
    : `${said} · score ${count(colleague.strength)}`;
}

function bandCell(bucket: string): HTMLElement {
  const band = BANDS.get(bucket);
  if (band === undefined) return el("span", "meta", bucket || "unknown");
  return strengthMeter(band[0], band[1]);
}

function colleagueRow(colleague: Colleague, position: number): HTMLElement {
  const row = el("li", "panel-row item item-colleague");
  row.append(
    el("span", "figure", String(position)),
    avatar(colleague.name, colleague.userID),
  );
  const main = el("div");
  main.append(
    el("div", "name", colleague.name),
    el("div", "meta", evidence(colleague)),
  );
  row.append(main, bandCell(colleague.bucket));
  return row;
}

/** The intro line. "Warmest first" is only true of a COMPLETE ranking: when the
 *  read stopped at its bound these are the warmest FOUND, and saying otherwise
 *  is the claim the tool itself refuses to make.
 *
 *  The contact is named, never identified: an id is what the product calls this
 *  row, not what the reader calls the human. Where the answer carries no name
 *  the line says "this contact" rather than falling back to the id. */
function intro(contactName: string, bounded: boolean): string {
  const who = contactName === "" ? "this contact" : contactName;
  return bounded
    ? `Warmest found first. More know ${who} than are listed, so this is not the whole network.`
    : `Who here knows ${who}, warmest first.`;
}

export function render(
  root: HTMLElement,
  data: unknown,
  warnings: Warning[],
): void {
  root.replaceChildren();
  if (data === null || data === undefined) {
    root.appendChild(
      el(
        "div",
        "empty empty-alone",
        "The host sent no structured result for this contact.",
      ),
    );
    return;
  }
  const answer = asRecord(data);
  const colleagues = known(answer);
  const bounded = warned(warnings, SWEEP_TRUNCATED);
  const map = panel("Who knows this contact", {
    level: "h1",
    action: el(
      "span",
      "meta",
      bounded
        ? `at least ${colleagues.length}`
        : `${colleagues.length} colleague${colleagues.length === 1 ? "" : "s"}`,
    ),
  });
  if (colleagues.length === 0) {
    map.appendChild(
      el(
        "p",
        "empty",
        "Nobody here has spoken to this contact. That is the answer, not a gap.",
      ),
    );
    root.appendChild(map);
    return;
  }
  const body = panelBody();
  body.appendChild(
    el("p", "intro", intro(asText(answer.contact_name), bounded)),
  );
  map.appendChild(body);
  const list = el("ol");
  colleagues.forEach((colleague, index) => {
    list.appendChild(colleagueRow(colleague, index + 1));
  });
  map.appendChild(list);
  root.appendChild(map);
}

onResult((data, warnings) => {
  const root = document.getElementById("root");
  // Guarded rather than asserted; see the account brief for why.
  if (root !== null) render(root, data, warnings);
});
