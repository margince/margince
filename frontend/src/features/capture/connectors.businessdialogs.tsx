// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../../api/schema";
import { TeamPicker } from "../../components/readerteam";
import { Field, TextInput } from "../../design-system/atoms";
import { CopyableText } from "../../design-system/clipboardcopy";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useFieldFaults } from "../../design-system/savepage";
import { useLocale, useT } from "../../i18n";
import { blankRequired, problemMessageOf } from "../../lib/problem";
import {
  type BusinessAccount,
  type BusinessDraft,
  type BusinessProvider,
  blankBusinessDraft,
  businessFields,
  businessTitles,
  changeBusinessVisibility,
  createBusinessAccount,
} from "./connectors.businessapi";
import { BusinessVisibilityDialog } from "./connectors.telegrambusiness.dialogs";

export function ConnectBusinessDialog({
  provider,
  teams,
  onClose,
  onDone,
}: Readonly<{
  provider: BusinessProvider;
  teams: readonly components["schemas"]["Team"][];
  onClose: () => void;
  onDone: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [draft, setDraft] = useState(blankBusinessDraft);
  const [token, setToken] = useState<string | null>(null);
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const connect = useMutation({
    mutationFn: (values: BusinessDraft) =>
      createBusinessAccount(provider, values),
    gcTime: 0,
    onSuccess: (verifyToken) => {
      connect.reset();
      setDraft(blankBusinessDraft);
      if (verifyToken) setToken(verifyToken);
      else onDone();
    },
  });
  return (
    <ConfirmModal
      open
      intent="form"
      onClose={token ? onDone : onClose}
      title={t(businessTitles[provider])}
      confirmLabel={
        token ? t("connectors.business.done") : t("connectors.business.connect")
      }
      unsaved={
        token !== null || Object.values(draft).some((value) => value !== "")
      }
      unsavedBody={token ? t("connectors.business.tokenDiscard") : undefined}
      bodyRef={faultsRoot}
      pending={connect.isPending}
      error={connect.isError ? problemMessageOf(connect.error, t) : null}
      onConfirm={() => {
        if (token) {
          onDone();
          return;
        }
        const required: Record<string, [string, string]> = {
          readerTeamId: [t("sharedMailboxes.readerTeam"), draft.readerTeamId],
        };
        for (const field of businessFields[provider])
          required[field.key] = [t(field.label), draft[field.key]];
        const blank = blankRequired(locale, required);
        if (!faults.show(blank)) connect.mutate(draft);
      }}
    >
      {token ? (
        <>
          <p>{t("connectors.business.verifyHelp")}</p>
          <CopyableText
            text={token}
            label={t("connectors.business.verifyToken")}
          />
        </>
      ) : (
        <div className="form-stack">
          <p>{t("connectors.business.connectHelp")}</p>
          {businessFields[provider].map((field) => (
            <Field
              key={field.key}
              label={t(field.label)}
              required
              error={faults.of(field.key)}
            >
              {(control) => (
                <TextInput
                  {...control}
                  type={field.secret ? "password" : "text"}
                  autoComplete={field.secret ? "new-password" : "off"}
                  value={draft[field.key]}
                  onChange={(event) => {
                    setDraft({ ...draft, [field.key]: event.target.value });
                    faults.edited(field.key);
                  }}
                />
              )}
            </Field>
          ))}
          <TeamPicker
            teams={teams}
            value={draft.readerTeamId}
            error={faults.of("readerTeamId")}
            onChange={(readerTeamId) => {
              setDraft({ ...draft, readerTeamId });
              faults.edited("readerTeamId");
            }}
          />
        </div>
      )}
    </ConfirmModal>
  );
}

export function BusinessReadersDialog({
  provider,
  account,
  onClose,
  onDone,
}: Readonly<{
  provider: BusinessProvider;
  account: BusinessAccount;
  onClose: () => void;
  onDone: () => void;
}>) {
  return (
    <BusinessVisibilityDialog
      owner={account.name}
      mailVisibility={account.mailVisibility}
      patch={(next) => changeBusinessVisibility(provider, account.id, next)}
      onClose={onClose}
      onDone={onDone}
    />
  );
}
