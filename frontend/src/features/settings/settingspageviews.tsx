// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, Suspense, useState } from "react";
import { PendingBody } from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { useT } from "../../i18n";
import { lazy } from "../../i18n/lazy";
import type { Route } from "../../lib/router";
import type { SettingsPageId } from "../../lib/settingscatalog";

type PageAt = Readonly<{ tab: string; route?: Route }>;

/** A lazy view inside a CardBoundary whose Retry loads its code again. */
export function renewable<Props extends object>(
  load: () => Promise<{ default: (props: Props) => ReactNode }>,
) {
  // React keeps a refused import on its lazy component for good, so a refusal
  // leaves a fresh one for the next mount and for Retry.
  const fresh = () =>
    lazy(async () => {
      try {
        return await load();
      } catch (error) {
        current = fresh();
        throw error;
      }
    });
  let current = fresh();
  return function RenewableView(props: Props) {
    const [View, setView] = useState(() => current);
    return (
      <CardBoundary onRetry={() => setView(() => current)}>
        <View {...props} />
      </CardBoundary>
    );
  };
}

/** One settings page whose code loads on its first visit, every tab at once. */
export function page<Parts>(
  load: () => Promise<Parts>,
  draw: (parts: Parts, at: PageAt) => ReactNode,
) {
  const View = renewable(async () => {
    const parts = await load();
    return { default: (at: PageAt) => draw(parts, at) };
  });
  // What one tab draws, as an element tree a test can walk without mounting.
  return Object.assign(View, {
    drawn: async (at: PageAt) => draw(await load(), at),
  });
}

/** What a page draws on `tab`; a tab the map does not name draws `first`. */
export function pick(
  tab: string,
  first: ReactNode,
  tabs: Readonly<Record<string, ReactNode>>,
): ReactNode {
  return Object.hasOwn(tabs, tab) ? tabs[tab] : first;
}

/** A page's body while its code arrives on the first visit; the frame stays. */
export function PageLoading({
  id,
  children,
}: Readonly<{ id: SettingsPageId; children: ReactNode }>) {
  const t = useT();
  return (
    <Suspense
      fallback={
        <PendingBody
          label={t("settings.pageLoading", {
            page: t(`settings.tab.${id}`),
          })}
          lines={6}
        />
      }
    >
      {children}
    </Suspense>
  );
}

// The pages drawn from this folder alone. settings.tsx holds the pages that
// import another feature, because the feature-boundary ledger names that file.
export const FOLDER_PAGE_VIEWS = {
  agents: page(
    () => import("./settings.aiaccess"),
    ({ AiAccessTab }) => <AiAccessTab />,
  ),

  // ---- members, teams, roles and seats ----
  members: page(
    () => import("./users-admin"),
    ({ UsersAdminCard }) => <UsersAdminCard />,
  ),
  // Its own page rather than a second card under members. Team Lead is
  // team-scoped, so this is also where an admin decides whose records a Team
  // Lead's membership hands over.
  teams: page(
    () => import("./users-access"),
    ({ TeamsCard }) => <TeamsCard />,
  ),
  // Who may merge a colleague's duplicate of a contact is a question of who may
  // change what.
  roles: page(
    () =>
      Promise.all([import("./roles-settings"), import("./sharedmergereviews")]),
    ([{ RolesSettings }, { SharedMergeReviewsCard }]) => (
      <>
        <RolesSettings />
        <SharedMergeReviewsCard />
      </>
    ),
  ),
  seats: page(
    () => import("./license"),
    ({ LicenseCard }) => <LicenseCard />,
  ),

  // ---- sales and data ----
  partners: page(
    () => import("./partnerstages"),
    ({ PartnerStagesCard }) => <PartnerStagesCard />,
  ),
  "web-forms": page(
    () => import("./webforms"),
    ({ WebFormsCard }) => <WebFormsCard />,
  ),

  // ---- compliance ----
  privacy: page(
    () => import("./settings.privacy"),
    ({ PrivacyLanes }, { tab }) => <PrivacyLanes tab={tab} />,
  ),
  jurisdiction: page(
    () =>
      Promise.all([
        import("./settings.jurisdiction"),
        import("./controllerparticulars"),
      ]),
    ([{ JurisdictionCard }, { ControllerParticularsCard }]) => (
      <>
        <JurisdictionCard />
        <ControllerParticularsCard />
      </>
    ),
  ),
  // Its own page: it answers to `audit_log`, where the privacy page answers to
  // consent and retention, and one reader can hold one without the other.
  audit: page(
    () => import("./settings-audit"),
    ({ AuditLogCard }) => <AuditLogCard />,
  ),

  // ---- system ----
  "system-health": page(
    () => import("./settings.systemhealth"),
    ({ SystemHealthPage }, { tab }) => <SystemHealthPage view={tab} />,
  ),
  configuration: page(
    () => import("./installation-configuration"),
    ({ InstallationConfigurationPage }) => <InstallationConfigurationPage />,
  ),
  // Its own page: a reindex, a queue reading and "empty the installation" were
  // three verbs on one screen, separated by nothing.
  reset: page(
    () => import("./settings.reset"),
    ({ ResetDataCard }) => <ResetDataCard />,
  ),
  about: page(
    () => import("./aboutpage"),
    ({ AboutPage }) => <AboutPage />,
  ),
};
