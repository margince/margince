// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowLeft, Lock } from "lucide-react";
import { type MouseEvent, useState } from "react";
import { SetupSteps } from "../../components/setup-steps";
import { AppLink } from "../../design-system/applink";
import { Badge, Button, EmptyState } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { RoleBadge } from "../../design-system/rbac";
import { useTooltip } from "../../design-system/tooltip";
import { useT } from "../../i18n";
import { opensElsewhere } from "../../lib/linkclick";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import {
  groupHeadingKey,
  readsOnly,
  SETTINGS_GROUPS,
  SETTINGS_PAGES,
  type SettingsGroupId,
  type SettingsPage,
  type SettingsReach,
} from "../../lib/settingscatalog";
import { settingsHref } from "../../lib/settingsrouting";
import { useSetupChecklist } from "../../lib/setup-checklist";
import { usePhoneViewport } from "../../lib/viewport";
import { PAGE_ICONS } from "./settingsnavicons";
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
  return event.button === 0 && !opensElsewhere(event);
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
        <AppLink
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
        </AppLink>
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
 * The settings landing page: every page this reader may open, by topic, then
 * who they are here.
 *
 * Built from the `settingsReach` partition the rail reads. A page the reader may
 * only consult is not in the rail, so this is where it stays reachable, marked
 * read-only.
 */
export function SettingsHome({ reach }: Readonly<{ reach: SettingsReach }>) {
  const t = useT();
  const me = useMe();
  const phone = usePhoneViewport();
  const openable = [...reach.acts, ...reach.looksUp];

  return (
    <div className="settings-stack arrive-stack">
      {/* The rail holds this box above phone width; below it this page is the
          directory, so the box comes here. */}
      {phone && <SettingsSearchBox pages={openable} />}

      <YourSetup reach={reach} />

      {groupsOf(openable).map(({ group, items }) => (
        <Panel key={group} title={t(groupHeadingKey(group))}>
          <PanelBody>
            <div className="settings-home-links">
              {items.map((page) => (
                <PageRow key={page.id} page={page} reach={reach} />
              ))}
            </div>
          </PanelBody>
        </Panel>
      ))}

      <Panel title={t("settings.home.access")}>
        <PanelBody>
          <QueryGate query={me} pendingLabel={t("settings.home.accessLoading")}>
            {(snapshot) => (
              /* Facts, not settings: a definition list rather than SettingRow,
                 which requires a control. The way to change any of it is to ask. */
              <dl className="settings-facts">
                <dt>{t("settings.home.rolesLabel")}</dt>
                <dd>
                  {/* The server's role keys, so a custom role shows the word a
                      reader would quote when asking for more. */}
                  <div className="settings-home-roles">
                    {(snapshot.roles ?? []).map((role) => (
                      <RoleBadge key={role} roleKey={role} />
                    ))}
                  </div>
                </dd>
                {/* An absent answer says nothing rather than guessing the
                    narrowest: a wrong claim about someone's access is worse. */}
                {snapshot.authorization && (
                  <>
                    <dt>{t("settings.home.seatLabel")}</dt>
                    <dd>
                      {t(
                        snapshot.authorization.seat_type === "full"
                          ? "settings.home.seat.full"
                          : "settings.home.seat.read",
                      )}
                    </dd>
                    <dt>{t("settings.home.reachLabel")}</dt>
                    <dd>
                      {t(
                        `settings.home.reach.${snapshot.authorization.row_scope}`,
                      )}
                    </dd>
                  </>
                )}
              </dl>
            )}
          </QueryGate>
        </PanelBody>
      </Panel>
    </div>
  );
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

/** The pages by group, both in the catalog's own order. */
function groupsOf(
  pages: readonly SettingsPage[],
): readonly { group: SettingsGroupId; items: readonly SettingsPage[] }[] {
  const open = new Set(pages);
  return SETTINGS_GROUPS.map((group) => ({
    group,
    items: SETTINGS_PAGES.filter(
      (page) => page.group === group && open.has(page),
    ),
  })).filter((entry) => entry.items.length > 0);
}

/** One row per page: its glyph, its name and what it is for. */
function PageRow({
  page,
  reach,
}: Readonly<{ page: SettingsPage; reach: SettingsReach }>) {
  const t = useT();
  const Glyph = PAGE_ICONS[page.id];
  const { ref, trigger, tip } = useTooltip<HTMLSpanElement>(
    t("settings.home.readOnly"),
  );
  // The tip opens over the lock for a pointer and on the row for a keyboard.
  // The hidden word already names the lock, so the tip adds no description.
  const {
    onPointerEnter,
    onPointerLeave,
    "aria-describedby": _spoken,
    ...onRowFocus
  } = trigger;
  const locked = readsOnly(reach, page.id);
  return (
    <AppLink
      className="settings-home-row"
      href={`#/settings/${page.id}`}
      {...(locked ? onRowFocus : {})}
      onClick={(event) => {
        if (!opensInThisTab(event)) {
          return;
        }
        event.preventDefault();
        navigate(settingsHref(page.id));
      }}
    >
      <Glyph size={16} aria-hidden className="settings-home-glyph" />
      <span className="settings-home-name">
        {t(`settings.tab.${page.id}`)}
        {locked && (
          <span
            ref={ref}
            className="settings-home-lock"
            onPointerEnter={onPointerEnter}
            onPointerLeave={onPointerLeave}
          >
            <Lock size={14} aria-hidden />
            <span className="sr-only">{t("settings.home.readOnly")}</span>
            {tip}
          </span>
        )}
      </span>
      <span className="t-caption">{t(`settings.page.${page.id}.sub`)}</span>
    </AppLink>
  );
}
