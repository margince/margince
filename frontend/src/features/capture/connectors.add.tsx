// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import type { components } from "../../api/schema";
import { Button, Modal } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Heading } from "../../design-system/heading";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { providerName } from "../../format/connectorprovider";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Provider = components["schemas"]["CaptureConnection"]["provider"];

// Mail and calendar are separate connections on both vendors, one consent
// each, so a member can bring one without the other.
export const ALL_PROVIDERS: readonly Provider[] = [
  "gmail",
  "gcal",
  "graph",
  "graphcal",
  "imap",
];

// The choice cannot be made from the names alone: only the OAuth mailboxes
// send, and IMAP serves every host with no OAuth.
const PROVIDER_BLURB: Record<Provider, MessageKey> = {
  gmail: "connectors.addGmailBrings",
  gcal: "connectors.addGcalBrings",
  graph: "connectors.addGraphBrings",
  graphcal: "connectors.addGraphCalBrings",
  imap: "connectors.addImapBrings",
  test_mailbox: "connectors.addTestMailboxBrings",
};

/** The card's create verb: one row per provider not yet connected. */
export function AddConnectionDialog({
  open,
  onClose,
  addable,
  pendingProvider,
  notConfigured501,
  connectError,
  onConnect,
  onImap,
  returnFocusTo,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  returnFocusTo?: () => HTMLElement | null;
  addable: readonly Provider[];
  pendingProvider: Provider | null;
  notConfigured501: Provider | null;
  connectError: string | null;
  onConnect: (provider: Provider) => void;
  onImap: () => void;
}>) {
  const t = useT();
  const headingId = useId();
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={headingId}
      intent="form"
      returnFocusTo={returnFocusTo}
    >
      <div className="form-stack">
        <Heading size="large" id={headingId} className="t-h2">
          {t("connectors.addConnection")}
        </Heading>
        <SettingList>
          {addable.map((provider) => (
            <SettingRow
              key={provider}
              testId={`connector-add-${provider}`}
              label={providerName(provider, t)}
              description={t(PROVIDER_BLURB[provider])}
              // Described by the row, but named by its provider: the row's
              // label would outrank `aria-label`, and every button would say "Connect".
              control={({ id, "aria-describedby": describedBy }) => (
                <Button
                  id={id}
                  aria-describedby={describedBy}
                  variant="ghost"
                  aria-label={t("connectors.connectProvider", {
                    provider: providerName(provider, t),
                  })}
                  pending={pendingProvider === provider}
                  onClick={
                    provider === "imap" ? onImap : () => onConnect(provider)
                  }
                >
                  {t("connectors.connect")}
                </Button>
              )}
            />
          ))}
        </SettingList>
        {notConfigured501 && (
          <Callout
            kind="outcome"
            title={t("connectors.providerNotConfigured", {
              provider: providerName(notConfigured501, t),
            })}
          />
        )}
        {connectError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("connectors.connectFailed")}
          >
            {connectError}
          </Callout>
        )}
      </div>
    </Modal>
  );
}
