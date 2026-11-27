// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEntityName } from "../components/entityref";
import type { Crumb } from "../design-system/breadcrumb";
import { useT } from "../i18n";
import { useOffer } from "../lib/offer.queries";
import type { Route } from "../lib/router";

export function useOfferLocation(route: Route) {
  const t = useT();
  const { data: offer } = useOffer(
    route.screen === "offers" ? route.id : undefined,
  );
  const { name } = useEntityName("deal", offer?.deal_id);
  if (!offer) return undefined;
  const crumbs: Crumb[] = [
    { label: t("nav.deals"), href: "#/deals" },
    { label: name || t("search.kind.deal"), href: `#/deals/${offer.deal_id}` },
    { label: offer.offer_number },
  ];
  return {
    crumbs,
    title: crumbs.map((crumb) => crumb.label).reverse() as [
      string,
      ...string[],
    ],
    heading: offer.offer_number,
  };
}
