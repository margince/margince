// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Checkbox,
  Field,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ChoiceList } from "../../design-system/choicelist";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { IMAP_PORT_DEFAULT, imapRefusal } from "../../lib/imapconnect";
import { labelFor } from "../../lib/labelfor";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { ImportWindowPicker } from "../../mail-history/window-picker";
import {
  checkedFields,
  credentialsOf,
  FIELD_LABEL,
  type FieldKey,
  fieldLabels,
  type Provider,
  providerFields,
  SECRETS,
  type Values,
} from "./sharedmailboxes.connect";

type SharedMailbox = components["schemas"]["SharedMailbox"];
type Team = components["schemas"]["Team"];
type Visibility = SharedMailbox["mail_visibility"];
type ConnectRequest = components["schemas"]["ConnectSharedMailboxRequest"];
type UpdateRequest = components["schemas"]["UpdateSharedMailboxRequest"];
type ImportWindow =
  components["schemas"]["SharedMailboxHistoryImportRequest"]["window"];

export const SHARED_MAILBOXES_KEY = ["shared-mailboxes"] as const;

export const VISIBILITY_LABEL: Record<Visibility, MessageKey> = {
  held: "sharedMailboxes.visibilityHeld",
  shared: "sharedMailboxes.visibilityShared",
};

const VISIBILITIES: readonly Visibility[] = ["held", "shared"];

const VISIBILITY_HINT: Record<Visibility, MessageKey> = {
  held: "sharedMailboxes.visibilityHeldHint",
  shared: "sharedMailboxes.visibilitySharedHint",
};

const PROVIDERS: readonly { value: Provider; label: MessageKey }[] = [
  { value: "imap", label: "firstRun.platform.imap" },
  { value: "graph", label: "firstRun.platform.microsoft" },
  { value: "gmail", label: "firstRun.platform.google" },
];

function useInvalidateMailboxes() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: SHARED_MAILBOXES_KEY });
}

function TeamPicker({
  teams,
  value,
  onChange,
  error,
}: Readonly<{
  teams: readonly Team[];
  value: string;
  onChange: (id: string) => void;
  error?: string;
}>) {
  const t = useT();
  return (
    <Field label={t("sharedMailboxes.readerTeam")} required error={error}>
      {(control) => (
        <Select
          {...control}
          value={value}
          onChange={onChange}
          placeholder={t("sharedMailboxes.readerTeamPlaceholder")}
          options={teams.map((team) => ({ value: team.id, label: team.name }))}
        />
      )}
    </Field>
  );
}

function VisibilityChoice({
  value,
  onChange,
}: Readonly<{ value: Visibility; onChange: (next: Visibility) => void }>) {
  const t = useT();
  return (
    <ChoiceList
      legend={t("sharedMailboxes.visibility")}
      value={value}
      onChange={onChange}
      choices={VISIBILITIES.map((choice) => ({
        value: choice,
        label: t(labelFor(VISIBILITY_LABEL, choice)),
        description: t(labelFor(VISIBILITY_HINT, choice)),
      }))}
    />
  );
}

/** One text field of the connect form: its label, value, setter and refusal. */
function CredentialField({
  label,
  value,
  onChange,
  error,
  secret = false,
  required = true,
}: Readonly<{
  label: string;
  value: string;
  onChange: (next: string) => void;
  error?: string;
  secret?: boolean;
  required?: boolean;
}>) {
  return (
    <Field label={label} required={required} error={error}>
      {(control) => (
        <TextInput
          {...control}
          type={secret ? "password" : "text"}
          autoComplete="off"
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </Field>
  );
}

function CredentialFields({
  provider,
  values,
  errorOf,
  set,
}: Readonly<{
  provider: Provider;
  values: Values;
  errorOf: (key: FieldKey) => string | undefined;
  set: (key: FieldKey, value: string) => void;
}>) {
  const t = useT();
  return providerFields(provider).map((key) =>
    key === "gmail.service_account_key" ? (
      <Field
        key={key}
        label={t(FIELD_LABEL[key])}
        required
        error={errorOf(key)}
      >
        {(control) => (
          <Textarea
            {...control}
            rows={6}
            value={values[key] ?? ""}
            onChange={(event) => set(key, event.target.value)}
          />
        )}
      </Field>
    ) : (
      <CredentialField
        key={key}
        label={t(FIELD_LABEL[key])}
        value={values[key] ?? ""}
        onChange={(next) => set(key, next)}
        error={errorOf(key)}
        secret={SECRETS.has(key)}
        required={key !== "imap.port"}
      />
    ),
  );
}

/** Connects a shared mailbox: its provider, address, reader team and login. */
export function ConnectSharedMailboxDialog({
  teams,
  onClose,
}: Readonly<{ teams: readonly Team[]; onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const invalidate = useInvalidateMailboxes();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const [provider, setProvider] = useState<Provider>("imap");
  const [visibility, setVisibility] = useState<Visibility>("held");
  const [values, setValues] = useState<Values>({
    "imap.port": String(IMAP_PORT_DEFAULT),
  });
  const connect = useMutation({
    mutationFn: async (body: ConnectRequest) =>
      unwrap(await api.POST("/shared-mailboxes", { body })),
    onSuccess: async () => {
      await invalidate();
      onClose();
    },
    // A failed connect keeps no secret on screen, as the personal form does.
    onError: () =>
      setValues((prior) => ({
        ...prior,
        "imap.secret": "",
        "graph.client_secret": "",
      })),
  });
  const labels = fieldLabels(t);
  const set = (key: FieldKey, value: string) => {
    setValues((prior) => ({ ...prior, [key]: value }));
    faults.edited(key);
  };
  const press = () => {
    if (faults.show(checkedFields(provider, values, labels, locale))) {
      connect.reset();
      return;
    }
    connect.mutate(
      {
        provider,
        address: (values.address ?? "").trim(),
        reader_team_id: values.reader_team_id ?? "",
        mail_visibility: visibility,
        ...credentialsOf(provider, values),
      },
      {
        onError: (error) => faults.show(imapRefusal(error, t, labels).byField),
      },
    );
  };
  return (
    <ConfirmModal
      open
      intent="drawer"
      onClose={onClose}
      title={t("sharedMailboxes.connectTitle")}
      confirmLabel={t("sharedMailboxes.connect")}
      pending={connect.isPending}
      bodyRef={faultsRoot}
      error={imapRefusal(connect.error, t, labels).rest}
      onConfirm={press}
    >
      <ChoiceList
        legend={t("sharedMailboxes.provider")}
        value={provider}
        onChange={setProvider}
        choices={PROVIDERS.map((choice) => ({
          value: choice.value,
          label: t(choice.label),
        }))}
      />
      <CredentialField
        label={t(FIELD_LABEL.address)}
        value={values.address ?? ""}
        onChange={(next) => set("address", next)}
        error={faults.of("address")}
      />
      <TeamPicker
        teams={teams}
        value={values.reader_team_id ?? ""}
        onChange={(id) => set("reader_team_id", id)}
        error={faults.of("reader_team_id")}
      />
      <VisibilityChoice value={visibility} onChange={setVisibility} />
      <CredentialFields
        provider={provider}
        values={values}
        errorOf={faults.of}
        set={set}
      />
    </ConfirmModal>
  );
}

/** Changes who reads a mailbox, or its visibility, for all its mail. */
export function ChangeSharedMailboxDialog({
  mailbox,
  teams,
  onClose,
}: Readonly<{
  mailbox: SharedMailbox;
  teams: readonly Team[];
  onClose: () => void;
}>) {
  const t = useT();
  const invalidate = useInvalidateMailboxes();
  const [team, setTeam] = useState(mailbox.reader_team_id);
  const [visibility, setVisibility] = useState<Visibility>(
    mailbox.mail_visibility,
  );
  const [narrowHistory, setNarrowHistory] = useState(false);
  const change = useMutation({
    mutationFn: async ({ id, body }: { id: string; body: UpdateRequest }) =>
      unwrap(
        await api.PATCH("/shared-mailboxes/{id}", {
          params: { path: { id } },
          body,
        }),
      ),
    onSuccess: async () => {
      await invalidate();
      onClose();
    },
  });
  const narrowing = visibility === "held" && mailbox.mail_visibility !== "held";
  const unchanged =
    team === mailbox.reader_team_id && visibility === mailbox.mail_visibility;
  return (
    <ConfirmModal
      open
      intent="form"
      onClose={onClose}
      title={t("sharedMailboxes.changeTitle", { address: mailbox.address })}
      confirmLabel={t("sharedMailboxes.changeConfirm")}
      confirmDisabled={unchanged}
      pending={change.isPending}
      error={change.isError ? problemMessageOf(change.error, t) : null}
      onConfirm={() =>
        change.mutate({
          id: mailbox.id,
          body: {
            confirm: true,
            ...(team !== mailbox.reader_team_id && { reader_team_id: team }),
            ...(visibility !== mailbox.mail_visibility && {
              mail_visibility: visibility,
            }),
            ...(narrowing && { apply_to_history: narrowHistory }),
          },
        })
      }
    >
      <div className="form-stack">
        <TeamPicker teams={teams} value={team} onChange={setTeam} />
        <VisibilityChoice value={visibility} onChange={setVisibility} />
        {narrowing && (
          <Checkbox
            label={t("sharedMailboxes.narrowHistory")}
            checked={narrowHistory}
            onChange={(event) => setNarrowHistory(event.target.checked)}
          />
        )}
        <Callout tone="warning" title={t("sharedMailboxes.changeWarning")} />
      </div>
    </ConfirmModal>
  );
}

/** Starts the import of a shared mailbox's past mail. */
export function SharedMailboxHistoryDialog({
  mailbox,
  onClose,
}: Readonly<{ mailbox: SharedMailbox; onClose: () => void }>) {
  const t = useT();
  const invalidate = useInvalidateMailboxes();
  const [span, setSpan] = useState<ImportWindow>("6m");
  const start = useMutation({
    mutationFn: async ({ id, chosen }: { id: string; chosen: ImportWindow }) =>
      unwrap(
        await api.POST("/shared-mailboxes/{id}/history-import", {
          params: { path: { id } },
          body: { window: chosen },
        }),
      ),
    onSuccess: async () => {
      await invalidate();
      onClose();
    },
  });
  return (
    <ConfirmModal
      open
      intent="form"
      onClose={onClose}
      title={t("sharedMailboxes.historyTitle", { address: mailbox.address })}
      confirmLabel={t("sharedMailboxes.historyStart")}
      pending={start.isPending}
      error={start.isError ? problemMessageOf(start.error, t) : null}
      onConfirm={() => start.mutate({ id: mailbox.id, chosen: span })}
    >
      <div className="form-stack">
        <p>{t("sharedMailboxes.historyIntro")}</p>
        <ImportWindowPicker value={span} onChange={setSpan} />
      </div>
    </ConfirmModal>
  );
}
