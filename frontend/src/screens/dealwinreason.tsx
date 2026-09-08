// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { IdentityFact } from "../design-system/identityline";
import { useT } from "../i18n";
import { WON_REASON_LABELS } from "./winreason";
import "./dealstatus.css";

type Deal = components["schemas"]["Deal"];

/**
 * The facts this component draws and no more — the same rule the identity line
 * it sits on follows. `status` is required on a Deal and optional here, because
 * every story and test that draws this builds the smallest deck that says
 * something, and absent reads as "not a won deal", which is what such a deck
 * means.
 */
export type WonWithoutContract = Partial<
  Pick<
    Deal,
    "status" | "won_without_contract_reason" | "won_without_contract_detail"
  >
>;

/**
 * How a won deal was won, when it was won without a contract.
 *
 * The server treats this answer as load-bearing — the whole justification for
 * letting the deal close without paperwork is that the gap becomes countable —
 * and until now no screen read it back. The rep who answered could not check
 * their own answer, and a won deal with paper looked identical to one without.
 *
 * On the identity line rather than in a pane, beside the `won` badge it
 * qualifies: it is a fact about the deal's standing, which is what this line
 * is for, and a reader asking "won on what?" is looking at the badge.
 *
 * Absent, never a placeholder, when the deal is not won or a contract carried
 * it. "Won with a contract" is the ordinary case and needs no sentence; a line
 * saying so on every won deal would bury the ones that need reading.
 */
export function WonWithoutContractFact({
  deal,
}: Readonly<{ deal: WonWithoutContract }>) {
  const t = useT();
  const reason = deal.won_without_contract_reason;
  if (deal.status !== "won" || !reason) {
    return null;
  }
  const label = t(WON_REASON_LABELS[reason]);
  // The detail replaces the label for `other`, and does not follow it:
  // "Something else: renewed on a handshake" says the category twice and
  // buries the only part written by a person. Every other reason is its own
  // answer and carries no detail.
  const detail = reason === "other" ? deal.won_without_contract_detail : null;
  if (!detail) {
    return <IdentityFact>{label}</IdentityFact>;
  }
  // Free text on a line of SHORT facts, so its WIDTH is bounded and its
  // content is not: it wraps inside a max-width rather than clipping.
  //
  // Two earlier spellings were worse. Trimmed in TS with the rest in a `title`
  // reads as solved and is not — a tooltip wants a mouse, is ignored by most
  // screen readers, and never appears for a keyboard or touch reader. Clipped
  // with no `title` is simply unreadable for everyone looking at it. Either way
  // the reader who loses is the person checking the words they just typed,
  // which is who this fact exists for.
  //
  // Wrapping is affordable because the server bounds the value: 500 characters
  // is a few lines, and the identity line already wraps.
  //
  // The rule is a class on this screen's own stylesheet rather than one in the
  // design system: one screen's free-text field is not a shape the shared
  // identity row owes everybody, and IdentityFact already takes a className for
  // exactly this.
  return <IdentityFact className="deal-win-detail">{detail}</IdentityFact>;
}
