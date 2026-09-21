// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { useRoute } from "../app/router";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

// What the connections card says about a consent round trip that has just come
// back. Its own file for the reason it was its own function first: the outcome
// table and the dismissal state are a self-contained answer to "what happened
// when you left this page", and the card below it is about standing decisions.

// The OAuth callback lands back on #/settings/connections/{outcome} — the
// route parses to id2 = "ok" | "denied" | "rejected" | "misconfigured" |
// "bad_client" | "error". Only these are server-defined (contract-first); any
// other value is silently ignored rather than rendering a raw route segment.
//
// Three of them exist so a failure nobody can fix by retrying does not tell the
// reader to retry, and they are three rather than two because the remedies are
// different screens: the provider refused the grant (reconnect and accept
// everything), its API was never enabled (the vendor console), or it refused
// this deployment's client credentials (the app card in Settings).
const OAUTH_OUTCOME_NOTE: Record<
  string,
  { key: MessageKey; tone: "success" | "danger" }
> = {
  ok: { key: "connectors.oauthOk", tone: "success" },
  denied: { key: "connectors.oauthDenied", tone: "danger" },
  rejected: { key: "connectors.oauthRejected", tone: "danger" },
  misconfigured: { key: "connectors.oauthMisconfigured", tone: "danger" },
  bad_client: { key: "connectors.oauthBadClient", tone: "danger" },
  error: { key: "connectors.oauthError", tone: "danger" },
};

/**
 * The OAuth return outcome: the callback lands back on
 * #/settings/connections/{outcome} — id2 on that route only, never parsed from
 * location.hash directly (the router already owns that).
 *
 * Dismissing (or navigating away, which unmounts it) clears it; the list itself
 * already refetches on mount, so "ok" needs no extra invalidation here. It sits
 * ABOVE the row list rather than in it: it reports on what the reader just did,
 * which is not one of the card's standing decisions.
 *
 * The heading answers the one thing a reader scans for — whether anything got
 * connected — and the outcome's own sentence under it says which remedy this
 * failure wants.
 */
export function OAuthOutcomeNote() {
  const t = useT();
  const route = useRoute();
  const oauthOutcome =
    route.screen === "settings" && route.id === "connections"
      ? route.id2
      : undefined;
  const [dismissedOutcome, setDismissedOutcome] = useState<string | null>(null);
  // Object.hasOwn, not a bare index: a route segment like "constructor" would
  // otherwise resolve to an inherited member and render an empty note.
  const note =
    oauthOutcome &&
    oauthOutcome !== dismissedOutcome &&
    Object.hasOwn(OAUTH_OUTCOME_NOTE, oauthOutcome)
      ? OAUTH_OUTCOME_NOTE[oauthOutcome]
      : undefined;
  if (!note) {
    return null;
  }
  return (
    <Callout
      tone={note.tone}
      kind="outcome"
      title={t(
        note.tone === "success"
          ? "connectors.oauthConnected"
          : "connectors.oauthNotConnected",
      )}
      dismiss={{
        label: t("connectors.dismissOutcome"),
        onDismiss: () => setDismissedOutcome(oauthOutcome ?? null),
      }}
    >
      {t(note.key)}
    </Callout>
  );
}
