// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import {
  Badge,
  Button,
  Checkbox,
  EmptyState,
  Field,
  Modal,
  OverflowMenu,
  TextInput,
} from "../design-system/atoms";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import { ScopeChips, scopeChipLabel } from "../design-system/passportselect";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  QueryStates,
  resetToSignedOut,
  throwProblem,
  WriteRefused,
} from "./common";
import { usePassportRevoke } from "./passport-revoke";
import { usePassports } from "./passports.queries";
import { MintedPassport, PassportUses } from "./settings.passportuse";
import "./settings.css";
import "./settings-agents.css";

const PASSPORT_SCOPES = ["read", "draft", "write", "send", "enrich"] as const;
type PassportScope = (typeof PASSPORT_SCOPES)[number];
type PassportSummary = components["schemas"]["PassportSummary"];

// Annotated, so an added scope is a missing-key compile error rather than a
// checkbox labelled with its wire token.
function scopeLabelKey(scope: PassportScope): MessageKey {
  return `passport.scope.${scope}`;
}

// The revoke confirm hands focus back to the passport's name: the row stays
// listed as revoked, but the menu item it was opened from is gone.
function passportAnchor(id: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-passport="${id}"]`);
}

export function PassportCard() {
  const t = useT();
  const [minting, setMinting] = useState(false);
  // Metadata only: PassportSummary carries no token, so this list cannot
  // re-disclose one.
  const list = usePassports();
  // A row carrying `connection` is a client's credential, listed by
  // ConnectedAgentsCard; the `oauth:` label prefix decides nothing.
  const minted = (list.data?.data ?? []).filter(
    (passport) => passport.connection == null,
  );

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
      {list.isSuccess && minted.length > 0 ? (
        <PassportTable passports={minted} onRevoke={revoke.ask} />
      ) : (
        <PanelBody>
          <QueryStates query={list} pendingLabel={t("settings.passports")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      <PanelBody>
        <p className="t-caption">{t("settings.passportsMcpHint")}</p>
      </PanelBody>
      <MintDialog
        open={minting}
        onClose={() => setMinting(false)}
        apiBaseUrl={list.data?.api_base_url}
        onMinted={() => list.refetch()}
      />
      {revoke.confirm}
    </Panel>
  );
}

function PassportTable({
  passports,
  onRevoke,
}: Readonly<{
  passports: readonly PassportSummary[];
  onRevoke: (id: string) => void;
}>) {
  const t = useT();
  const dates = usePassportDates();
  const columns: DataTableColumn<PassportSummary>[] = [
    {
      key: "name",
      header: t("settings.passportColName"),
      grow: true,
      fold: "title",
      render: (passport) => (
        <PassportName passport={passport} caption={dates.caption(passport)} />
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
              label={t("settings.passportActions", { name: passport.label })}
            >
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
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const lastUsed = (passport: PassportSummary) =>
    passport.last_used_at
      ? formatDate(passport.last_used_at, locale, recordZone)
      : t("settings.passportNeverUsed");
  const expires = (passport: PassportSummary) =>
    passport.expires_at
      ? formatDate(passport.expires_at, locale, viewerZone())
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

function MintDialog({
  open,
  onClose,
  apiBaseUrl,
  onMinted,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  apiBaseUrl: string | undefined;
  onMinted: () => unknown;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [label, setLabel] = useState("");
  const [scopes, setScopes] = useState<Set<PassportScope>>(
    new Set(["read", "draft"]),
  );
  const tokenRegion = useRef<HTMLDivElement | null>(null);
  const titleId = useId();
  const formId = useId();
  const scopeHintId = useId();

  const mint = useMutation({
    mutationFn: async (request: { label: string; scopes: PassportScope[] }) => {
      const { data, error, response } = await api.POST("/passports", {
        body: { label: request.label.trim() || null, scopes: request.scopes },
      });
      // An expired session must not read as a mint that did nothing. Drop every
      // cached answer before the probe re-asks, so the next sign-in sees none.
      if (response.status === 401) {
        await resetToSignedOut(queryClient);
      }
      if (error) {
        throwProblem(error);
      }
      if (!response.ok) {
        throwProblem({
          type: "about:blank",
          title: response.statusText || "Request failed",
          status: response.status,
        });
      }
      return data;
    },
    onSuccess: () => onMinted(),
  });

  // The token is shown once and never re-served, so focus goes to it.
  const minted = mint.isSuccess;
  useEffect(() => {
    if (minted) {
      tokenRegion.current?.focus();
    }
  }, [minted]);

  const close = () => {
    onClose();
    setLabel("");
    setScopes(new Set(["read", "draft"]));
    mint.reset();
  };
  // Refused mid-flight, as `mint.reset()` does not cancel the POST. Once the
  // token shows, Done is the only exit: anything else would lose it.
  const dismiss = () => {
    if (!mint.isPending && !mint.isSuccess) {
      close();
    }
  };

  return (
    <Modal
      open={open}
      onClose={dismiss}
      closeDisabled={mint.isPending || mint.isSuccess}
      labelledBy={titleId}
      intent="form"
    >
      <Heading size="large" className="t-h2 modal-title" id={titleId}>
        {t("settings.mint")}
      </Heading>
      {/* Mounted for the dialog's whole life: a live region inserted with its
          content is not reliably announced. */}
      <div className="passport-token" ref={tokenRegion} tabIndex={-1}>
        <div role="status">
          {mint.isSuccess && <p>{t("settings.passportCreated")}</p>}
        </div>
        {mint.isSuccess && (
          <MintedPassport token={mint.data.token} apiBaseUrl={apiBaseUrl} />
        )}
      </div>
      {!mint.isSuccess && (
        <form
          id={formId}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            if (scopes.size > 0 && !mint.isPending) {
              mint.mutate({ label, scopes: [...scopes] });
            }
          }}
        >
          <Field label={t("settings.passportLabel")}>
            {(control) => (
              <TextInput
                {...control}
                value={label}
                onChange={(event) => setLabel(event.target.value)}
              />
            )}
          </Field>
          <fieldset
            className="field-multiselect"
            aria-describedby={scopeHintId}
          >
            <legend className="t-name">{t("settings.passportScopes")}</legend>
            <p id={scopeHintId} className="t-caption">
              {t("settings.passportScopesHint")}
            </p>
            {PASSPORT_SCOPES.map((scope) => (
              <Checkbox
                key={scope}
                checked={scopes.has(scope)}
                onChange={(event) => {
                  const next = new Set(scopes);
                  if (event.target.checked) {
                    next.add(scope);
                  } else {
                    next.delete(scope);
                  }
                  setScopes(next);
                }}
                label={t(scopeLabelKey(scope))}
              />
            ))}
          </fieldset>
          <WriteRefused titleKey="settings.mintFailed" error={mint.error} />
        </form>
      )}
      <div className="actions">
        {mint.isSuccess ? (
          // Open on success: closing would take the one sight of the token.
          <Button variant="primary" onClick={close}>
            {t("settings.mintDone")}
          </Button>
        ) : (
          <>
            <Button disabled={mint.isPending} onClick={dismiss}>
              {t("settings.mintCancel")}
            </Button>
            <Button
              type="submit"
              form={formId}
              variant="primary"
              reason={
                scopes.size === 0 && !mint.isPending
                  ? t("settings.passportScopesRequired")
                  : undefined
              }
              pending={mint.isPending}
              busyLabel={t("settings.minting")}
            >
              {t("settings.mint")}
            </Button>
          </>
        )}
      </div>
    </Modal>
  );
}
