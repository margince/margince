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
 * The frame of one settings page, chosen by the address.
 *
 * Addresses are flat: `#/settings/audit`, not `#/settings/admin/audit`. A group
 * segment would tie a bookmark to the group, so moving a page would break it.
 * `settingsRouteTarget` rewrites every older address once on arrival.
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
  // The sidebar lists the pages and the shell names the page, so this frame has
  // no nav column or heading. It sets the spacing itself, because pages hold
  // forms and tables, not only cards, and `.wrap > .card + .card` misses those.
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
      {/* The unsaved-draft guard sits above the routed screen (App.tsx). One
          here would unmount with the screen and lose a draft when the reader
          leaves settings. The cards hand their drafts to the page's save bar. */}
      {tabs.length > 1 && (
        <SettingsTabStrip page={page} tabs={tabs} current={tab} />
      )}
      <SettingsSavePage>
        <Frame id={page.id}>
          <div key={tab} className="settings-stack arrive-stack">
            {/* Shown once, and only on a page where the reader can change
              nothing. Without it they would have to infer that from disabled
              cards. */}
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
