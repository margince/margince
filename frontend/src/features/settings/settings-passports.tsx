// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../../api/schema";
import { PassportAreasDrawer } from "../../components/passport-areas";
import { usePassportRevoke } from "../../components/passport-revoke";
import {
  offersWorkAreas,
  type WorkArea,
  WorkAreasCell,
} from "../../components/workareas";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../../design-system/panel";
import { ScopeChips, scopeChipLabel } from "../../design-system/passportselect";
import { QueryStates } from "../../design-system/querystates";
import { formatDate, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { usePassports } from "../../lib/passports.queries";
import {
  PassportUsageLine,
  usePassportUsageDetail,
} from "../automations/agentusage";
import { PassportUses } from "./settings.passportuse";
import { MintDialog } from "./settings-passports-mint";
import "./settings.css";
import { useViewerZone } from "../../lib/viewerzone";
import "./settings-agents.css";

type PassportSummary = components["schemas"]["PassportSummary"];

// Only a live passport is offered a change of areas. The server would take one
// on a dead passport too, where it changes nothing anyone can use.
function isLive(passport: PassportSummary, now: number): boolean {
  return (
    passport.revoked_at == null &&
    (passport.expires_at == null || Date.parse(passport.expires_at) > now)
  );
}

// The revoke confirm hands focus back to the passport's name. The row stays
// listed as revoked, but the menu item it was opened from is gone.
function passportAnchor(id: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-passport="${id}"]`);
}

export function PassportCard() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [minting, setMinting] = useState(false);
  // Metadata only: PassportSummary carries no token, so this list cannot
  // re-disclose one.
  const list = usePassports();
  // A row carrying `connection` is a client's credential, listed by
  // ConnectedAgentsCard; the `oauth:` label prefix decides nothing.
  const minted = (list.data?.data ?? []).filter(
    (passport) => passport.connection == null,
  );
  const reachable = list.data?.reachable_areas ?? [];
  const [changingAreas, setChangingAreas] = useState<PassportSummary | null>(
    null,
  );
  // A revoked passport can do nothing more, so it waits behind a toggle. One
  // revoked from this view stays, struck, so focus has its row to land on.
  const [showRevoked, setShowRevoked] = useState(false);
  const [revokedHere, setRevokedHere] = useState<ReadonlySet<string>>(
    new Set(),
  );
  const hidden = minted.filter(
    (passport) => passport.revoked_at != null && !revokedHere.has(passport.id),
  );
  const shown = showRevoked
    ? minted
    : minted.filter((passport) => !hidden.includes(passport));
  const askRevoke = (id: string) => {
    setRevokedHere((ids) => new Set(ids).add(id));
    revoke.ask(id);
  };

  const revoke = usePassportRevoke({
    verb: "settings.revoke",
    question: "settings.revokeConfirm",
    focusAfter: passportAnchor,
  });

  return (
    <Panel
      title={t("settings.passports")}
      titleAction={
        <Button onClick={() => setMinting(true)}>
          {t("settings.mintOpen")}
        </Button>
      }
    >
      <PanelBody>
        <PanelIntro>{t("settings.passportsSub")}</PanelIntro>
        <PassportUses apiBaseUrl={list.data?.api_base_url} />
      </PanelBody>
      <PanelGroupHead title={t("settings.passportsYours")} level="h3" />
      {list.isSuccess && shown.length > 0 ? (
        <PassportTable
          passports={shown}
          reachable={reachable}
          onRevoke={askRevoke}
          onChangeAreas={setChangingAreas}
        />
      ) : list.isSuccess && hidden.length > 0 ? null : (
        <PanelBody>
          <QueryStates query={list} pendingLabel={t("settings.passports")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      <PanelBody className="form-stack">
        {hidden.length > 0 && (
          <span>
            <Button variant="link" onClick={() => setShowRevoked((on) => !on)}>
              {showRevoked
                ? t("settings.passportsHideRevoked")
                : plural("settings.passportsShowRevoked", hidden.length, {
                    count: formatNumber(hidden.length, locale),
                  })}
            </Button>
          </span>
        )}
        <p className="t-caption">{t("settings.passportsMcpHint")}</p>
      </PanelBody>
      <MintDialog
        open={minting}
        onClose={() => setMinting(false)}
        apiBaseUrl={list.data?.api_base_url}
        reachable={reachable}
        onMinted={() => list.refetch()}
      />
      {changingAreas && (
        <PassportAreasDrawer
          key={changingAreas.id}
          subject={{
            id: changingAreas.id,
            name: changingAreas.label,
            areas: changingAreas.areas,
          }}
          reachable={reachable}
          onClose={() => setChangingAreas(null)}
        />
      )}
      {revoke.confirm}
    </Panel>
  );
}

function PassportTable({
  passports,
  reachable,
  onRevoke,
  onChangeAreas,
}: Readonly<{
  passports: readonly PassportSummary[];
  reachable: readonly WorkArea[];
  onRevoke: (id: string) => void;
  onChangeAreas: (passport: PassportSummary) => void;
}>) {
  const t = useT();
  const dates = usePassportDates();
  // Read once: a passport that expires while the page is open is refused by
  // the server, and the drawer says so.
  const [now] = useState(() => Date.now());
  const areas = offersWorkAreas(reachable);
  const usageDetail = usePassportUsageDetail<PassportSummary>(
    (passport) => passport.label,
  );
  const columns: DataTableColumn<PassportSummary>[] = [
    {
      key: "name",
      header: t("settings.passportColName"),
      grow: true,
      fold: "title",
      render: (passport) => (
        <span className="agents-name-usage">
          <PassportName passport={passport} caption={dates.caption(passport)} />
          <PassportUsageLine passport={passport} />
        </span>
      ),
    },
    {
      key: "scopes",
      header: t("settings.passportColScopes"),
      render: (passport) => (
        <span className="agents-scopes">
          <ScopeChips
            labels={passport.scopes.map((scope) => scopeChipLabel(t, scope))}
          />
        </span>
      ),
    },
    ...(areas
      ? [
          {
            key: "areas",
            header: t("settings.passportColAreas"),
            render: (passport: PassportSummary) => (
              <span className="agents-scopes">
                <WorkAreasCell areas={passport.areas} />
              </span>
            ),
          },
        ]
      : []),
    {
      key: "used",
      header: t("settings.passportColLastUsed"),
      fold: "hide",
      render: (passport) => dates.lastUsed(passport),
    },
    {
      key: "expires",
      header: t("settings.passportColExpires"),
      fold: "hide",
      render: (passport) => dates.expires(passport),
    },
    {
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (passport) =>
        passport.revoked_at == null && (
          <span className="cell-actions">
            <OverflowMenu
              label={t("table.rowActions", { name: passport.label })}
            >
              {areas && isLive(passport, now) && (
                <Button
                  aria-label={t("passportAreas.openNamed", {
                    name: passport.label,
                  })}
                  aria-haspopup="dialog"
                  onClick={() => onChangeAreas(passport)}
                >
                  {t("passportAreas.open")}
                </Button>
              )}
              <Button
                variant="danger"
                aria-label={t("settings.revokeNamed", { name: passport.label })}
                aria-haspopup="dialog"
                onClick={() => onRevoke(passport.id)}
              >
                {t("settings.revoke")}
              </Button>
            </OverflowMenu>
          </span>
        ),
    },
  ];
  return (
    <DataTable
      bleed
      fold
      label={t("settings.passportsYours")}
      columns={columns}
      rows={[...passports]}
      rowKey={(passport) => passport.id}
      detail={usageDetail}
    />
  );
}

// The name truncates rather than wraps; its title carries the whole of it. A
// revoked passport is struck, never dimmed, so it keeps the AA contrast floor.
function PassportName({
  passport,
  caption,
}: Readonly<{ passport: PassportSummary; caption: string }>) {
  const t = useT();
  const revoked = passport.revoked_at != null;
  return (
    <span className="agents-name" data-passport={passport.id} tabIndex={-1}>
      <span className="agents-name-line">
        <span
          className={
            revoked ? "agents-name-text agents-ended" : "agents-name-text"
          }
          title={passport.label}
        >
          {passport.label}
        </span>
        {revoked && <Badge tone="danger">{t("settings.revoked")}</Badge>}
      </span>
      {/* The folded row hides the date columns; their headings still read them. */}
      <span className="t-caption agents-fold-caption" aria-hidden="true">
        {caption}
      </span>
    </span>
  );
}

// When a passport was last used is a record of something that happened, so it
// reads on the record zone. Its expiry is the holder's own deadline.
function usePassportDates() {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const lastUsed = (passport: PassportSummary) =>
    passport.last_used_at
      ? formatDate(passport.last_used_at, locale, readerZone)
      : t("settings.passportNeverUsed");
  const expires = (passport: PassportSummary) =>
    passport.expires_at
      ? formatDate(passport.expires_at, locale, readerZone)
      : t("settings.passportNoExpiry");
  const caption = (passport: PassportSummary) =>
    [
      passport.last_used_at
        ? t("settings.passportLastUsedOn", { date: lastUsed(passport) })
        : t("settings.passportNeverUsedCaption"),
      passport.expires_at
        ? t("settings.passportExpiresOn", { date: expires(passport) })
        : t("settings.passportNoExpiry"),
    ].join(" · ");
  return { lastUsed, expires, caption };
}
