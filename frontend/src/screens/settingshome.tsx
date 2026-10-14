// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowLeft } from "lucide-react";
import { type MouseEvent, useState } from "react";
import { useSetupChecklist } from "../app/setup-checklist";
import { SetupSteps } from "../app/setup-reminder";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { type Fact, FactList } from "../design-system/factlist";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelRow,
} from "../design-system/panel";
import { RoleBadge } from "../design-system/rbac";
import { useT } from "../i18n";
import { navigate } from "../lib/router";
import { useMe } from "../lib/session";
import type {
  SettingsGroupId,
  SettingsPage,
  SettingsReach,
} from "./settingscatalog";
import { SETTINGS_GROUPS } from "./settingscatalog";
import { settingsHref } from "./settingsrouting";
import { SettingsSearchBox } from "./settingssearchbox";

/**
 * Whether this click is the plain one an SPA may answer itself.
 *
 * A modified click is the reader asking the BROWSER for something — a new tab,
 * a new window, a download — and `preventDefault` on it silently takes that
 * away. So the SPA answers the unmodified primary click and lets every other
 * one through to the href, which is why these rows are anchors rather than
 * buttons in the first place.
 */
function opensInThisTab(event: MouseEvent<HTMLAnchorElement>): boolean {
  return (
    event.button === 0 &&
    !event.metaKey &&
    !event.ctrlKey &&
    !event.shiftKey &&
    !event.altKey
  );
}

/**
 * What a reader is told when the address names a page that is not theirs.
 *
 * Two kinds, and they are different facts. `denied` means the page exists and
 * this reader may not open it; `unknown` means nothing answers that address at
 * all. Saying "not found" to the first would be a lie the reader could disprove
 * by asking a colleague, and saying "not yours" to the second would invent a
 * page.
 *
 * Neither names WHICH grant is missing. The address is already in the URL bar
 * and the reader can quote it; naming the object would tell someone who cannot
 * open the page exactly what to ask to be given, which is a disclosure the
 * page itself refuses to make.
 */
export function SettingsBoundary({
  kind,
}: Readonly<{ kind: "denied" | "unknown" }>) {
  const t = useT();
  return (
    <EmptyState
      title={t(
        kind === "denied"
          ? "settings.boundary.deniedTitle"
          : "settings.boundary.unknownTitle",
      )}
      action={
        // The catalogued `.link-button`, not a second primitive. A real anchor
        // so the row can be opened in a new tab, middle-clicked and copied like
        // any other link — and `navigate` on top of it so the SPA does not
        // reload the app to move one address.
        <a
          className="link-button"
          href={`#${routeHashOf()}`}
          onClick={(event) => {
            if (!opensInThisTab(event)) {
              return;
            }
            event.preventDefault();
            navigate(settingsHref());
          }}
        >
          <ArrowLeft />
          {/* The rail's OWN word for this address, not a second one: the link
              goes exactly where the sidebar's first row goes, and a place named
              twice is a place a reader has to work out is one place. */}
          {t("settings.home")}
        </a>
      }
    >
      {t(
        kind === "denied"
          ? "settings.boundary.deniedBody"
          : "settings.boundary.unknownBody",
      )}
    </EmptyState>
  );
}

// The home address as a hash, for the anchor's href. Spelled here rather than
// interpolated at the call site so the link and the click handler cannot drift
// into pointing at two different addresses.
function routeHashOf(): string {
  const route = settingsHref();
  return `/${route.screen}`;
}

/**
 * The settings landing page.
 *
 * Four questions, in the order a reader asks them: what is mine, what can I
 * change, what can I only look at, and who am I here. The middle two are the
 * `settingsReach` partition — the same one the sidebar is built from — so a page
 * cannot be in the rail and missing here, or listed here as manageable while its
 * every control is disabled.
 *
 * The "look up" panel is load-bearing rather than a courtesy. The rail carries
 * only pages a reader can act on, so this is where a page they may read and not
 * change goes on living: without it, dropping a page from the rail would read as
 * taking it away.
 */
export function SettingsHome({ reach }: Readonly<{ reach: SettingsReach }>) {
  const t = useT();
  const personal = reach.acts.filter((page) => page.group === "me");
  const manageable = groupsOf(reach.acts.filter((page) => page.group !== "me"));
  const consultable = groupsOf(reach.looksUp);
  const accessFacts = useAccessFacts();

  return (
    <div className="settings-stack arrive-stack">
      {/* At every viewport, not only where the rail is. The rail owns the
          desktop search box and the rail is gone at phone width, so this is the
          one a reader on a phone reaches. */}
      <SettingsSearchBox pages={[...reach.acts, ...reach.looksUp]} />

      <YourSetup reach={reach} />

      <Panel title={t("settings.home.yours")}>
        <PageRows pages={personal} />
      </Panel>

      {manageable.length > 0 && (
        <Panel title={t("settings.home.manage")}>
          {manageable.map(({ group, items }) => (
            <GroupRows key={group} group={group} items={items} />
          ))}
        </Panel>
      )}

      {consultable.length > 0 && (
        <Panel title={t("settings.home.lookUp")}>
          {consultable.map(({ group, items }) => (
            <GroupRows key={group} group={group} items={items} />
          ))}
        </Panel>
      )}

      <Panel title={t("settings.home.access")}>
        <PanelBody>
          {/* Facts, not settings: nothing here is operable, and the way to
              change any of it is to ask. */}
          <FactList facts={accessFacts} />
        </PanelBody>
      </Panel>
    </div>
  );
}

/**
 * Who the reader is here. Roles come as the server resolved them, so a custom
 * role shows the key they would quote when asking for more.
 */
function useAccessFacts(): readonly Fact[] {
  const t = useT();
  const me = useMe();
  const roles: Fact = {
    key: "roles",
    term: t("settings.home.rolesLabel"),
    value: (
      <div className="settings-home-roles">
        {(me.data?.roles ?? []).map((role) => (
          <RoleBadge key={role} roleKey={role} />
        ))}
      </div>
    ),
  };
  const authorization = me.data?.authorization;
  // Absent while /me is in flight: say nothing rather than guess the narrowest.
  if (!authorization) {
    return [roles];
  }
  return [
    roles,
    {
      key: "seat",
      term: t("settings.home.seatLabel"),
      value: t(
        authorization.seat_type === "full"
          ? "settings.home.seat.full"
          : "settings.home.seat.read",
      ),
    },
    {
      key: "reach",
      term: t("settings.home.reachLabel"),
      value: t(`settings.home.reach.${authorization.row_scope}`),
    },
  ];
}

/**
 * The reader's own setup steps, the ones the top bar reminds them of. Hiding
 * that reminder leaves this card alone: here the steps are always one look away.
 */
function YourSetup({ reach }: Readonly<{ reach: SettingsReach }>) {
  const t = useT();
  const setup = useSetupChecklist(reach);
  const [showDone, setShowDone] = useState(false);
  if (!setup.settled || setup.total === 0) {
    return null;
  }
  const complete = setup.done === setup.total;
  return (
    <Panel
      title={t("settings.home.setup.title")}
      titleAction={<Badge>{t("settings.scope.self")}</Badge>}
    >
      <PanelBody>
        {complete && (
          <Callout
            tone="success"
            title={t("settings.home.setup.complete")}
            actions={
              showDone ? undefined : (
                <Button onClick={() => setShowDone(true)}>
                  {t("settings.home.setup.show")}
                </Button>
              )
            }
          />
        )}
        {(!complete || showDone) && (
          <SetupSteps
            steps={setup.steps}
            count={{ done: setup.done, total: setup.total }}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

/** The pages of one half, bucketed by group in the catalog's own order. */
function groupsOf(
  pages: readonly SettingsPage[],
): readonly { group: SettingsGroupId; items: readonly SettingsPage[] }[] {
  return SETTINGS_GROUPS.map((group) => ({
    group,
    items: pages.filter((page) => page.group === group),
  })).filter((entry) => entry.items.length > 0);
}

function GroupRows({
  group,
  items,
}: Readonly<{ group: SettingsGroupId; items: readonly SettingsPage[] }>) {
  const t = useT();
  return (
    <>
      <PanelGroupHead title={t(`settings.group.${group}`)} level="h3" />
      <PageRows pages={items} />
    </>
  );
}

/**
 * One row per page: its name, what it is for, and whose state it changes, so
 * the home is more than a second copy of the menu. Anchors, so a row opens in
 * a new tab like any link.
 */
function PageRows({ pages }: Readonly<{ pages: readonly SettingsPage[] }>) {
  const t = useT();
  return (
    <>
      {pages.map((page) => (
        <PanelRow key={page.id} interactive className="settings-home-row">
          <a
            className="settings-home-press"
            href={`#/settings/${page.id}`}
            onClick={(event) => {
              if (!opensInThisTab(event)) {
                return;
              }
              event.preventDefault();
              navigate(settingsHref(page.id));
            }}
          >
            <span className="settings-home-rowhead">
              <span>{t(`settings.tab.${page.id}`)}</span>
              <Badge>{t(`settings.scope.${page.scope}`)}</Badge>
            </span>
            <span className="t-caption">
              {t(`settings.page.${page.id}.sub`)}
            </span>
          </a>
        </PanelRow>
      ))}
    </>
  );
}
