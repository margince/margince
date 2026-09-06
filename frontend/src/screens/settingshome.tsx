// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowLeft } from "lucide-react";
import type { MouseEvent } from "react";
import { navigate } from "../app/router";
import { EmptyState } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { RoleBadge } from "../design-system/rbac";
import { useT } from "../i18n";
import { useMe } from "./common";
import type { SettingsPage } from "./settingscatalog";
import { SETTINGS_GROUPS } from "./settingscatalog";
import { settingsHref } from "./settingsrouting";

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
          {t("settings.boundary.back")}
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
 * Three questions, in the order a reader asks them: what is mine to change,
 * what else may I reach, and who am I here. The first two are built from the
 * SAME visible-page list the sidebar renders, so a page can never appear in one
 * and not the other — which is what a second hand-maintained list would have
 * guaranteed eventually.
 */
export function SettingsHome({
  pages,
}: Readonly<{ pages: readonly SettingsPage[] }>) {
  const t = useT();
  const me = useMe();
  const personal = pages.filter((page) => page.group === "me");
  const rest = SETTINGS_GROUPS.filter((group) => group !== "me")
    .map((group) => ({
      group,
      items: pages.filter((page) => page.group === group),
    }))
    .filter((entry) => entry.items.length > 0);

  return (
    <div className="settings-stack arrive-stack">
      <Panel title={t("settings.home.yours")}>
        <PanelBody>
          <PageLinks pages={personal} />
        </PanelBody>
      </Panel>

      {rest.map(({ group, items }) => (
        <Panel key={group} title={t(`settings.group.${group}`)}>
          <PanelBody>
            <PageLinks pages={items} />
          </PanelBody>
        </Panel>
      ))}

      <Panel title={t("settings.home.access")}>
        <PanelBody>
          {/* The roles as the server resolved them, not as a role name this
              screen guessed. A reader with a custom role sees its key, which is
              the word they would quote when asking for more. */}
          <div className="settings-home-roles">
            {(me.data?.roles ?? []).map((role) => (
              <RoleBadge key={role} roleKey={role} />
            ))}
          </div>
        </PanelBody>
      </Panel>
    </div>
  );
}

function PageLinks({ pages }: Readonly<{ pages: readonly SettingsPage[] }>) {
  const t = useT();
  return (
    <div className="settings-home-links">
      {pages.map((page) => (
        <a
          key={page.id}
          className="link-button"
          href={`#/settings/${page.id}`}
          onClick={(event) => {
            if (!opensInThisTab(event)) {
              return;
            }
            event.preventDefault();
            navigate(settingsHref(page.id));
          }}
        >
          {t(`settings.tab.${page.id}`)}
        </a>
      ))}
    </div>
  );
}
