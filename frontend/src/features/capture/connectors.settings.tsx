// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useCaptureSettings } from "../../components/capture-settings";
import { Checkbox, Modal } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { isMailbox, providerName } from "../../format/connectorprovider";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  errorClassKey,
  missingCalendarWriteGrant,
  missingSendGrant,
} from "../../lib/connector-status";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { ConnectorContextTagRow } from "./connectors.contexttag";
import { HistoryBaselineRow } from "./connectors.historybaseline";

type CaptureConnection = components["schemas"]["CaptureConnection"];
type MailPosture = NonNullable<CaptureConnection["mail_posture"]>;

// A drawer: a `SettingList` row cannot open in place, and these settings carry a
// select with its own confirm.
export function ConnectionDrawer({
  conn,
  open,
  onClose,
}: Readonly<{
  conn: CaptureConnection;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const accountId = useId();
  const connected = conn.status === "connected";
  const mailbox = isMailbox(conn.provider);
  // The account names the drawer only when its line renders.
  const accountRef = conn.account_label ? ` ${accountId}` : "";
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={`${titleId}${accountRef}`}
      intent="drawer"
    >
      <DrawerHead>
        <Heading size="large" id={titleId} className="modal-title">
          {providerName(conn.provider, t)}
        </Heading>
        {conn.account_label && (
          <p className="t-label connector-account" id={accountId}>
            {conn.account_label}
          </p>
        )}
      </DrawerHead>
      <DrawerBody>
        <div className="form-stack">
          <ConnectorFacts conn={conn} />
          {connected && (
            <SettingList testId={`connector-${conn.provider}-settings`}>
              {mailbox && <MailPostureRow conn={conn} />}
              {mailbox && <SignatureEnrichmentRow conn={conn} />}
              {mailbox && <HistoryBaselineRow conn={conn} />}
              <ConnectorContextTagRow conn={conn} />
            </SettingList>
          )}
        </div>
      </DrawerBody>
    </Modal>
  );
}

function ConnectorFacts({ conn }: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const { locale } = useLocale();
  const at = (iso: string) => formatDateTime(iso, locale, viewerZone());
  const failing = conn.status === "error" || conn.status === "reauth_required";
  return (
    <ul className="connector-facts">
      <li>
        {conn.last_synced_at
          ? t("connectors.lastSynced", { at: at(conn.last_synced_at) })
          : t("connectors.neverSynced")}
      </li>
      {conn.next_sync_due_at && (
        <li>{t("connectors.nextCheck", { at: at(conn.next_sync_due_at) })}</li>
      )}
      <li>
        {conn.watch_expires_at
          ? t("connectors.pushRenewal", { at: at(conn.watch_expires_at) })
          : t("connectors.polled")}
      </li>
      {failing && (
        <li>
          <ErrorLine inline standing>
            {t(errorClassKey(conn.last_sync_error_class))}
          </ErrorLine>
        </li>
      )}
      {missingCalendarWriteGrant(conn) && (
        <li>{t("scheduling.readOnlyCalendar")}</li>
      )}
      {missingSendGrant(conn) && <li>{t("connectors.reconnectToSend")}</li>}
    </ul>
  );
}

// Only a narrowing has anything to offer captured mail: the server refuses to
// widen history as a side effect, and `apply_to_history` only ever narrows.
const postureRank: Record<MailPosture, number> = {
  shared: 0,
  classified: 1,
  held: 2,
};

// A Select, not a Switch: three postures, and `shared` stays offered but
// refused so a reader learns it exists and who can allow it.
function MailPostureRow({ conn }: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const settings = useCaptureSettings();
  const save = useSetMailPosture(conn.provider);
  const [pendingPosture, setPendingPosture] = useState<MailPosture | null>(
    null,
  );
  const [applyToHistory, setApplyToHistory] = useState(false);
  const sharedAllowed = settings.data?.shared_posture_allowed ?? false;
  const posture = conn.mail_posture ?? "classified";
  const help = t(`connectors.mailPosture.help.${posture}` as MessageKey);

  const choose = (next: MailPosture) => {
    if (postureRank[next] > postureRank[posture]) {
      setPendingPosture(next);
      return;
    }
    save.mutate({ posture: next, applyToHistory: false });
  };
  const closeConfirm = () => {
    setPendingPosture(null);
    setApplyToHistory(false);
  };

  return (
    <>
      <SettingRow
        testId={`connector-${conn.provider}-mail-posture`}
        label={t("connectors.mailPosture.label")}
        description={
          sharedAllowed
            ? help
            : `${help} ${t("connectors.mailPosture.sharedNeedsAdmin")}`
        }
        control={(controlProps) => (
          <Select
            {...controlProps}
            value={posture}
            disabled={save.isPending}
            onChange={(next) => choose(next as MailPosture)}
            options={[
              {
                value: "classified",
                label: t("connectors.mailPosture.classified"),
              },
              { value: "held", label: t("connectors.mailPosture.held") },
              {
                value: "shared",
                label: t("connectors.mailPosture.shared"),
                disabled: !sharedAllowed,
              },
            ]}
          />
        )}
      />
      <ConfirmModal
        open={pendingPosture !== null}
        onClose={closeConfirm}
        title={t("connectors.mailPosture.historyTitle")}
        confirmLabel={t("connectors.mailPosture.historyConfirm")}
        pending={save.isPending}
        error={save.error ? problemMessageOf(save.error, t) : undefined}
        onConfirm={() => {
          if (pendingPosture) {
            save.mutate(
              { posture: pendingPosture, applyToHistory },
              { onSuccess: closeConfirm },
            );
          }
        }}
      >
        <p>{t("connectors.mailPosture.historyBody")}</p>
        <Checkbox
          checked={applyToHistory}
          onChange={(e) => setApplyToHistory(e.target.checked)}
          label={t("connectors.mailPosture.historyApply")}
        />
      </ConfirmModal>
    </>
  );
}

function useSetMailPosture(provider: CaptureConnection["provider"]) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      posture: MailPosture;
      applyToHistory: boolean;
    }) => {
      unwrap(
        await api.PUT("/connectors/{provider}/mail-posture", {
          params: { path: { provider } },
          body: {
            posture: vars.posture,
            apply_to_history: vars.applyToHistory,
          },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["connectors"] });
    },
  });
}

// Tri-state on the wire, two on screen: the description says whether the
// answer is this mailbox's own or the workspace's default.
function SignatureEnrichmentRow({
  conn,
}: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const settings = useCaptureSettings();
  const save = useSetSignatureEnrichment(conn.provider);
  const workspaceDefault = settings.data?.signature_enrich ?? true;
  const own = conn.signature_enrich_enabled;
  return (
    <SettingRow
      testId={`connector-${conn.provider}-signature-enrich`}
      label={t("connectors.signatureEnrich.label")}
      description={
        own === null || own === undefined
          ? t("connectors.signatureEnrich.followingDefault")
          : t("connectors.signatureEnrich.ownAnswer")
      }
      control={
        <Switch
          testId={`connector-${conn.provider}-signature-enrich-toggle`}
          label={t("connectors.signatureEnrich.label")}
          labelHidden
          checked={own ?? workspaceDefault}
          disabled={save.isPending}
          onChange={(next) => save.mutate(next)}
        />
      }
    />
  );
}

function useSetSignatureEnrichment(provider: CaptureConnection["provider"]) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (enabled: boolean) => {
      unwrap(
        await api.PUT("/connectors/{provider}/signature-enrichment", {
          params: { path: { provider } },
          body: { enabled },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["connectors"] });
    },
  });
}
