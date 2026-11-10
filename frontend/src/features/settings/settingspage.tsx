// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Callout } from "../../design-system/callout";
import { useT } from "../../i18n";
import {
  type ReviewActions,
  ReviewActionsContext,
  useReviewActions,
} from "../../lib/reviewactions";
import type { Route } from "../../lib/router";
import type { SettingsPageId } from "../../lib/settingscatalog";
import { settingsRouteTarget } from "../../lib/settingsrouting";
import { SIGNATURE_REVIEW_PARAM } from "../../lib/signaturereviewlink";
import { useUrlParams } from "../../lib/urlstate";
import { RetainedSignaturePanel } from "./retainedsignature";
import { useSettingsDestination } from "./settingsdestination";
import { SettingsBoundary, SettingsHome } from "./settingshome";
import { useSettingsReach } from "./settingsnav";
import { SettingsSavePage } from "./settingssavepage";
import { SettingsTabStrip } from "./settingstabs";

/**
 * The settings screen: one page of the catalog, chosen by the address.
 *
 * Flat addresses now — `#/settings/audit`, not `#/settings/admin/audit`. The
 * group segment said which HALF of settings a reader was in, which was a fact
 * about who the page was for rather than about the page, so moving one between
 * groups moved its bookmark too. `settingsRouteTarget` still answers every
 * address the product minted, rewriting it once on arrival.
 */
export function SettingsPage({
  route,
  reviewActions,
  content,
  Frame,
}: Readonly<{
  route: Route;
  reviewActions?: ReviewActions;
  // The cards each page draws, and what a page keeps across its tabs. The
  // settings screen passes both, so this frame imports no feature's cards.
  content: (id: SettingsPageId, tab: string, route: Route) => ReactNode;
  Frame: (props: { id: SettingsPageId; children: ReactNode }) => ReactNode;
}>) {
  const inheritedReviewActions = useReviewActions();
  const [params] = useUrlParams();
  const t = useT();
  const address = useSettingsDestination(route);
  const reach = useSettingsReach();
  const signatureReview = params.get(SIGNATURE_REVIEW_PARAM);
  // A linked controller review has its own authority, separate from privacy settings.
  const target = settingsRouteTarget(route);
  if (
    signatureReview &&
    target.kind === "page" &&
    target.page === "privacy" &&
    (target.tab === undefined || target.tab === "requests")
  ) {
    return (
      <div className="wrap">
        <SettingsSavePage>
          <RetainedSignaturePanel id={signatureReview} />
        </SettingsSavePage>
      </div>
    );
  }
  // No nav column and no heading of its own: the entries are the sidebar's second
  // level now, and the shell's page head names the entry, so the page is that
  // entry's own content across the whole reading column.
  //
  // Every page sets its rhythm HERE rather than relying on the shell's
  // `.wrap > .card + .card` default, because that rule matches a card following a
  // card and settings pages are no longer stacks of cards: the merge left several
  // holding a `<form>`, a `<section>`, a flex wrapper or a bare heading-plus-table.
  // Where the rule missed, the gap was ZERO and two surfaces read as one. Owning
  // it once is the difference between every page spacing correctly and every
  // page having to remember to. Width is owned the same way and is the same for
  // all of them, so there is nothing here to branch on.
  if (address.kind === "home") {
    return (
      <div className="wrap">
        <SettingsHome reach={reach} />
      </div>
    );
  }
  if (address.kind === "boundary") {
    return (
      <div className="wrap">
        <SettingsBoundary kind={address.boundary} />
      </div>
    );
  }
  const { page, tabs, tab } = address;

  return (
    <div className="wrap">
      {/* Unsaved drafts in here are held by the guard above the routed screen
          (App.tsx), not by this screen. A guard installed HERE could only see
          moves between settings entries: it unmounts with the screen, so a draft
          was safe from one tab to the next and still discarded without a word the
          moment the reader clicked Contacts. The cards below hand their drafts
          to the page's one save bar, which claims for them. */}
      {tabs.length > 1 && (
        <SettingsTabStrip page={page} tabs={tabs} current={tab} />
      )}
      <SettingsSavePage>
        <Frame id={page.id}>
          <div key={tab} className="settings-stack arrive-stack">
            {/* Said once, at the top, and only for a page with no open control: the
              `looksUp` half of the partition. A reader who can change nothing would
              otherwise infer it from a screenful of disabled cards, and a banner over
              a control that works is worse than silence. */}
            {address.readOnly && (
              <Callout kind="standing" title={t("settings.readOnlyPageTitle")}>
                {t("settings.readOnlyPage")}
              </Callout>
            )}
            <ReviewActionsContext
              value={reviewActions ?? inheritedReviewActions}
            >
              {content(page.id, tab, route)}
            </ReviewActionsContext>
          </div>
        </Frame>
      </SettingsSavePage>
    </div>
  );
}
