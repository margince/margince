// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowLeft } from "lucide-react";
import type { MouseEvent } from "react";
import { navigate } from "../app/router";
import { Badge, EmptyState } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { RoleBadge } from "../design-system/rbac";
import { useT } from "../i18n";
import { useMe } from "./common";
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
  const me = useMe();
  const personal = reach.acts.filter((page) => page.group === "me");
  const manageable = groupsOf(reach.acts.filter((page) => page.group !== "me"));
  const consultable = groupsOf(reach.looksUp);

  return (
    <div className="settings-stack arrive-stack">
      {/* At every viewport, not only where the rail is. The rail owns the
          desktop search box and the rail is gone at phone width, so this is the
          one a reader on a phone reaches. */}
      <SettingsSearchBox pages={[...reach.acts, ...reach.looksUp]} />

      <Panel title={t("settings.home.yours")}>
        <PanelBody>
          <PageRows pages={personal} />
        </PanelBody>
      </Panel>

      {manageable.length > 0 && (
        <Panel
          title={t("settings.home.manage")}
          sub={t("settings.home.manageSub")}
        >
          <PanelBody>
            {manageable.map(({ group, items }) => (
              <GroupRows key={group} group={group} items={items} />
            ))}
          </PanelBody>
        </Panel>
      )}

      {consultable.length > 0 && (
        <Panel
          title={t("settings.home.lookUp")}
          sub={t("settings.home.lookUpSub")}
        >
          <PanelBody>
            {consultable.map(({ group, items }) => (
              <GroupRows key={group} group={group} items={items} />
            ))}
          </PanelBody>
        </Panel>
      )}

      <Panel title={t("settings.home.access")}>
        <PanelBody>
          {/* Facts, not settings: a definition list rather than SettingRow,
              which requires a control because every row of it is something a
              reader can operate. Nothing here is operable — this panel answers
              "who am I here", and the way to change any of it is to ask. */}
          <dl className="settings-facts">
            <dt>{t("settings.home.rolesLabel")}</dt>
            <dd>
              {/* The roles as the server resolved them, not as a role name this
                  screen guessed. A reader with a custom role sees its key,
                  which is the word they would quote when asking for more. */}
              <div className="settings-home-roles">
                {(me.data?.roles ?? []).map((role) => (
                  <RoleBadge key={role} roleKey={role} />
                ))}
              </div>
            </dd>
            {/* Seat and reach in words. Both are on the snapshot already and
                neither was shown: a reader told "read seat" by a disabled
                button has to guess whether it is their seat, their role or a
                bug. `authorization` is absent while /me is in flight, and an
                absent answer says nothing rather than guessing the narrowest —
                a wrong claim about someone's own access is worse than a gap. */}
            {me.data?.authorization && (
              <>
                <dt>{t("settings.home.seatLabel")}</dt>
                <dd>
                  {t(
                    me.data.authorization.seat_type === "full"
                      ? "settings.home.seat.full"
                      : "settings.home.seat.read",
                  )}
                </dd>
                <dt>{t("settings.home.reachLabel")}</dt>
                <dd>
                  {t(`settings.home.reach.${me.data.authorization.row_scope}`)}
                </dd>
              </>
            )}
          </dl>
        </PanelBody>
      </Panel>
    </div>
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
    <section className="settings-home-group">
      <h3 className="t-caption settings-home-groupname">
        {t(`settings.group.${group}`)}
      </h3>
      <PageRows pages={items} />
    </section>
  );
}

/**
 * One row per page: its name, what it is for, and whose state it changes.
 *
 * A title-only link was the whole defect this replaces — it made the home a
 * second copy of the menu, so a reader who could not tell two pages apart in
 * the rail could not tell them apart here either. The subtitle and the scope
 * are already in the catalog and were being thrown away.
 */
function PageRows({ pages }: Readonly<{ pages: readonly SettingsPage[] }>) {
  const t = useT();
  return (
    <div className="settings-home-links">
      {pages.map((page) => (
        <a
          key={page.id}
          className="settings-home-row"
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
            <span className="settings-home-rowname">
              {t(`settings.tab.${page.id}`)}
            </span>
            <Badge quiet>{t(`settings.scope.${page.scope}`)}</Badge>
          </span>
          <span className="t-caption">{t(`settings.page.${page.id}.sub`)}</span>
        </a>
      ))}
    </div>
  );
}
