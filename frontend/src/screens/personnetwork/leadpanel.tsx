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
import { Panel } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { evidenceSentence } from "../personroutes";

type RouteCandidate = components["schemas"]["PersonGraphRouteCandidate"];

/**
 * LeadPanel names the recommended route and offers the ask.
 *
 * The head of the ranked list, not a second opinion about it: the server
 * ranks, and this panel draws `routes[0]`. Two components each deciding what
 * "best" means is how the strip and the list start disagreeing on screen.
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
  blocked: string | undefined;
  onAsk: (route: RouteCandidate) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <Panel
      tone="accent"
      title={
        route.through_display_name
          ? t("person.graph.routeVia", {
              name: route.via_display_name,
              through: route.through_display_name,
            })
          : t("person.graph.routeDirect", { name: route.via_display_name })
      }
      sub={t("person.intro.leadEyebrow")}
      titleAction={
        <Badge tone={blocked ? undefined : "success"} quiet={Boolean(blocked)}>
          {blocked ?? t("person.intro.leadRouteBadge")}
        </Badge>
      }
      actions={
        blocked ? null : (
          <Button onClick={() => onAsk(route)}>
            {t("person.intro.askFirstName", { name: route.via_display_name })}
          </Button>
        )
      }
    >
      {/* The chain, drawn left to right: who asks, through whom, to whom. A
          reader checks the shape of a route before they read its counts. */}
      <div className="pn-hero">
        <Avatar name={route.via_display_name} size="md" />
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
          <Badge quiet>{t("person.intro.factOneSided")}</Badge>
        )}
        <Badge quiet>
          {route.through_display_name
            ? t("person.intro.factIndirect")
            : t("person.intro.factDirect")}
        </Badge>
        {/* Receipts are the messages behind the claim, and only a direct route
            carries them: pooled counts are disclosable where the
            correspondence itself is not. */}
        {route.receipts && route.receipts.length > 0 ? (
          <Badge quiet>
            {t("person.intro.factReceipts", {
              count: formatNumber(route.receipts.length, locale),
            })}
          </Badge>
        ) : null}
      </div>
    </Panel>
  );
}
