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
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor } from "../../lib/labelfor";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { ImportWindowPicker } from "../../mail-history/window-picker";

type SharedMailbox = components["schemas"]["SharedMailbox"];
type Team = components["schemas"]["Team"];
type Visibility = SharedMailbox["mail_visibility"];
type Provider =
  components["schemas"]["ConnectSharedMailboxRequest"]["provider"];
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
}: Readonly<{
  teams: readonly Team[];
  value: string;
  onChange: (id: string) => void;
}>) {
  const t = useT();
  return (
    <Field label={t("sharedMailboxes.readerTeam")} required>
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

/** One text field of a credential: the label, the value, and its setter. */
function CredentialField({
  label,
  value,
  onChange,
  secret = false,
}: Readonly<{
  label: string;
  value: string;
  onChange: (next: string) => void;
  secret?: boolean;
}>) {
  return (
    <Field label={label} required>
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

type Credentials = Record<string, string>;

const REQUIRED_CREDENTIALS: Record<Provider, readonly string[]> = {
  imap: ["host", "username", "secret"],
  graph: ["tenant_id", "client_id", "client_secret"],
  gmail: ["service_account_key"],
};

function credentialsOf(
  provider: Provider,
  values: Credentials,
): Partial<ConnectRequest> {
  const value = (key: string) => values[key] ?? "";
  switch (provider) {
    case "imap":
      return {
        imap: {
          host: value("host").trim(),
          port: Number(value("port") || "993"),
          username: value("username").trim(),
          secret: value("secret"),
          mailbox: "INBOX",
          max_messages: 50,
        },
      };
    case "graph":
      return {
        graph: {
          tenant_id: value("tenant_id").trim(),
          client_id: value("client_id").trim(),
          client_secret: value("client_secret"),
        },
      };
    case "gmail":
      return { gmail: { service_account_key: value("service_account_key") } };
    default:
      return {};
  }
}

function CredentialFields({
  provider,
  values,
  set,
}: Readonly<{
  provider: Provider;
  values: Credentials;
  set: (key: string, value: string) => void;
}>) {
  const t = useT();
  const field = (key: string, label: MessageKey, secret = false) => (
    <CredentialField
      key={key}
      label={t(label)}
      value={values[key] ?? ""}
      onChange={(next) => set(key, next)}
      secret={secret}
    />
  );
  switch (provider) {
    case "imap":
      return (
        <>
          {field("host", "connectors.imapHost")}
          {field("port", "connectors.imapPort")}
          {field("username", "connectors.imapUsername")}
          {field("secret", "connectors.imapSecret", true)}
        </>
      );
    case "graph":
      return (
        <>
          {field("tenant_id", "sharedMailboxes.graphTenant")}
          {field("client_id", "sharedMailboxes.graphClient")}
          {field("client_secret", "sharedMailboxes.graphSecret", true)}
        </>
      );
    case "gmail":
      return (
        <Field label={t("sharedMailboxes.gmailKey")} required>
          {(control) => (
            <Textarea
              {...control}
              rows={6}
              value={values.service_account_key ?? ""}
              onChange={(event) =>
                set("service_account_key", event.target.value)
              }
            />
          )}
        </Field>
      );
    default:
      return null;
  }
}

/** Connects a shared mailbox: its provider, address, reader team and login. */
export function ConnectSharedMailboxDialog({
  teams,
  onClose,
}: Readonly<{ teams: readonly Team[]; onClose: () => void }>) {
  const t = useT();
  const invalidate = useInvalidateMailboxes();
  const [provider, setProvider] = useState<Provider>("imap");
  const [address, setAddress] = useState("");
  const [team, setTeam] = useState("");
  const [visibility, setVisibility] = useState<Visibility>("held");
  const [values, setValues] = useState<Credentials>({ port: "993" });
  const connect = useMutation({
    mutationFn: async (body: ConnectRequest) =>
      unwrap(await api.POST("/shared-mailboxes", { body })),
    onSuccess: async () => {
      await invalidate();
      onClose();
    },
    // A refused login keeps no secret on screen for the next try.
    onError: () =>
      setValues((prior) => ({ ...prior, secret: "", client_secret: "" })),
  });
  const ready =
    address.trim() !== "" &&
    team !== "" &&
    REQUIRED_CREDENTIALS[provider].every(
      (key) => (values[key] ?? "").trim() !== "",
    );
  return (
    <ConfirmModal
      open
      intent="drawer"
      onClose={onClose}
      title={t("sharedMailboxes.connectTitle")}
      confirmLabel={t("sharedMailboxes.connect")}
      confirmDisabled={!ready}
      pending={connect.isPending}
      error={connect.isError ? problemMessageOf(connect.error, t) : null}
      onConfirm={() =>
        connect.mutate({
          provider,
          address: address.trim(),
          reader_team_id: team,
          mail_visibility: visibility,
          ...credentialsOf(provider, values),
        })
      }
    >
      <div className="form-stack">
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
          label={t("sharedMailboxes.address")}
          value={address}
          onChange={setAddress}
        />
        <TeamPicker teams={teams} value={team} onChange={setTeam} />
        <VisibilityChoice value={visibility} onChange={setVisibility} />
        <CredentialFields
          provider={provider}
          values={values}
          set={(key, value) =>
            setValues((prior) => ({ ...prior, [key]: value }))
          }
        />
      </div>
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
