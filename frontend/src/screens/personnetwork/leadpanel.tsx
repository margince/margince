// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one move this page recommends, and the evidence behind it.
//
// The page's ONLY accent panel. Accent means "this is the move", so a second
// tinted panel would make the reader choose which recommendation to believe.
// The indigo AI treatment is a different thing entirely and never appears
// here: nothing on this panel is machine-authored.

import type { components } from "../../api/schema";
import { Avatar, Badge, Button } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { Panel } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { evidenceSentence, lastContactBucket } from "../personroutes";
import { ReceiptList } from "./receipts";

type RouteCandidate = components["schemas"]["PersonGraphRouteCandidate"];
type RouteEvidence = components["schemas"]["PersonGraphRouteEvidence"];
type Translate = ReturnType<typeof useT>;

/**
 * LeadPanel names the recommended route and offers the ask.
 *
 * The head of the ranked list, not a second opinion about it: the server
 * ranks, and this panel draws `routes[0]`. Two components each deciding what
 * "best" means is how the strip and the list start disagreeing on screen.
 *
 * Two halves: the move on the left — who asks whom, in a sentence and a
 * drawn chain — and the counts it rests on to the right, so a reader who
 * trusts the sentence acts on it and one who does not checks the figures
 * without leaving the panel.
 */
export function LeadPanel({
  route,
  targetName,
  blocked,
  onAsk,
}: Readonly<{
  route: RouteCandidate;
  targetName: string;
  // Why the ask cannot be made, when it cannot. A panel that offered a button
  // answering 409 would be a control that exists to fail.
  blocked: string | null;
  onAsk: (route: RouteCandidate) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <Panel
      tone="accent"
      title={verdict(route, t)}
      sub={t("person.intro.leadEyebrow")}
      titleAction={
        <Badge tone={blocked ? undefined : "success"} quiet={Boolean(blocked)}>
          {blocked ?? t("person.intro.leadRouteBadge")}
        </Badge>
      }
    >
      <div className="pn-verdict">
        <div className="pn-verdict-main">
          {/* The chain, drawn left to right: who asks, through whom, to whom.
              A reader checks the shape of a route before they read its
              counts. */}
          <div className="pn-hero">
            <Avatar
              name={route.via_display_name}
              identity={route.via_user_id}
              size="md"
            />
            <div className="pn-hero-who">
              <strong>{route.via_display_name}</strong>
              <p>
                {route.through_display_name
                  ? t("person.intro.heroIndirect", {
                      through: route.through_display_name,
                    })
                  : t("person.intro.heroDirect")}
              </p>
            </div>
            <span className="pn-hero-arrow" aria-hidden="true">
              →
            </span>
            <Avatar name={targetName} size="md" />
            <div className="pn-hero-who">
              <strong>{targetName}</strong>
            </div>
          </div>

          <p className="pn-counts">
            {evidenceSentence(route.evidence, t, plural, locale)}
          </p>

          <div className="pn-facts">
            {route.evidence.two_way ? (
              <Badge quiet>{t("person.intro.factReciprocal")}</Badge>
            ) : (
              <Badge tone="warn">{t("person.intro.factOneSided")}</Badge>
            )}
            <Badge quiet>
              {route.through_display_name
                ? t("person.intro.factIndirect")
                : t("person.intro.factDirect")}
            </Badge>
          </div>

          {/* The ask sits with the move, not in a band under both halves:
              the button answers the sentence above it, and the figures to
              the right are what a reader checks before pressing it. */}
          {blocked ? null : (
            <p className="pn-verdict-actions">
              <Button variant="primary" onClick={() => onAsk(route)}>
                {t("person.intro.askFirstName", {
                  name: route.via_display_name,
                })}
              </Button>
            </p>
          )}
        </div>

        <EvidencePlate route={route} targetName={targetName} />
      </div>
    </Panel>
  );
}

/**
 * verdict is the panel's headline: the move, with the one fact that earns it.
 *
 * Three sentences for three shapes of route. A one-sided route is named as
 * such in the headline rather than only in a badge below, because "already
 * write to each other" over thirty unanswered sends would be the lie the
 * evidence line exists to prevent.
 */
function verdict(route: RouteCandidate, t: Translate): string {
  const name = route.via_display_name;
  if (route.through_display_name) {
    return t("person.intro.verdictVia", {
      name,
      through: route.through_display_name,
    });
  }
  return route.evidence.two_way
    ? t("person.intro.verdictDirect", { name })
    : t("person.intro.verdictOneSided", { name });
}

/**
 * EvidencePlate is the figures the sentence was written from.
 *
 * Receipts are the messages behind the claim, and only a direct route
 * carries them: pooled counts are disclosable where the correspondence
 * itself is not, so a route through a colleague says so instead of showing
 * an empty list that would read as "no messages".
 */
function EvidencePlate({
  route,
  targetName,
}: Readonly<{ route: RouteCandidate; targetName: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const ev = route.evidence;
  const receipts = route.receipts ?? [];
  return (
    <div className="pn-evidence">
      <Eyebrow as="h3">{t("person.intro.evidenceEyebrow")}</Eyebrow>
      <div className="pn-readings">
        <div className="pn-reading">
          <b>
            {formatNumber(ev.interactions_90d, locale)}
            <small>{t("person.intro.evidenceWindow")}</small>
          </b>
          <span>{t("person.intro.evidenceExchanges")}</span>
          <ExchangeSplit
            evidence={ev}
            viaName={route.via_display_name}
            targetName={targetName}
          />
        </div>
        <div className="pn-reading">
          <b>{lastContactReading(ev, t, locale)}</b>
          <span>{t("person.intro.evidenceLastContact")}</span>
        </div>
      </div>
      {receipts.length > 0 ? (
        <div className="pn-evidence-receipts">
          <Eyebrow as="h4">
            {t("person.intro.factReceipts", {
              count: formatNumber(receipts.length, locale),
            })}
          </Eyebrow>
          <ReceiptList receipts={receipts} />
        </div>
      ) : (
        <p className="pn-counts">{t("person.graph.countsOnly")}</p>
      )}
    </div>
  );
}

/**
 * ExchangeSplit shows how much of the traffic came from each end.
 *
 * Drawn only when the server split the count: a bar guessed from a total
 * alone would show a share nothing measured. The figures are in words under
 * it, so the bar is never the only carrier.
 */
function ExchangeSplit({
  evidence,
  viaName,
  targetName,
}: Readonly<{
  evidence: RouteEvidence;
  viaName: string;
  targetName: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const inbound = evidence.inbound_90d;
  const outbound = evidence.outbound_90d;
  if (inbound === undefined || outbound === undefined) {
    return null;
  }
  const total = Math.max(inbound + outbound, 1);
  return (
    <>
      <span className="pn-split" aria-hidden="true">
        <i className="pn-split-in" style={{ width: pct(inbound, total) }} />
        <i className="pn-split-out" style={{ width: pct(outbound, total) }} />
      </span>
      <span className="pn-split-legend">
        <span className="pn-split-key-in">
          {t("person.intro.evidenceFrom", {
            count: formatNumber(inbound, locale),
            name: targetName,
          })}
        </span>
        <span className="pn-split-key-out">
          {t("person.intro.evidenceFrom", {
            count: formatNumber(outbound, locale),
            name: viaName,
          })}
        </span>
      </span>
    </>
  );
}

function pct(part: number, total: number): string {
  return `${(part / total) * 100}%`;
}

// The reading-sized spelling of the fact the routes rows put in a sentence.
// Both read the bucket from `lastContactBucket`, so the two sizes cannot
// disagree about which day counts as yesterday.
function lastContactReading(
  ev: RouteEvidence,
  t: Translate,
  locale: Locale,
): string {
  const last = lastContactBucket(ev);
  switch (last.kind) {
    case "never":
      return t("person.intro.lastNever");
    case "today":
      return t("person.intro.lastToday");
    case "yesterday":
      return t("person.intro.lastYesterday");
    case "days":
      return t("person.intro.lastDays", {
        days: formatNumber(last.days, locale),
      });
  }
}
