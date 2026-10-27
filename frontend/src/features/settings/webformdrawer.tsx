// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { ifMatch } from "../../api/version";
import {
  Button,
  Checkbox,
  Field,
  Modal,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { ChoiceList } from "../../design-system/choicelist";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  problemFieldErrorsOf,
  problemMessageOf,
  unwrap,
} from "../../lib/problem";
import {
  pastesSecret,
  WEB_FORM_SENDERS,
  WEB_FORMS_KEY,
  type WebForm,
  type WebFormCreated,
  type WebFormSender,
} from "./webformkinds";

export type WebFormDraft = Readonly<{
  name: string;
  sender: WebFormSender;
  origins: string;
  senderSecret: string;
  consentRequired: boolean;
  consentWording: string;
  consentPolicyVersion: string;
  redirectUrl: string;
  successMessage: string;
}>;

export function draftFor(form?: WebForm): WebFormDraft {
  return {
    name: form?.name ?? "",
    sender: form?.sender ?? "browser",
    origins: (form?.allowed_origins ?? []).join("\n"),
    senderSecret: "",
    consentRequired: form?.consent_required ?? false,
    consentWording: form?.consent_wording ?? "",
    consentPolicyVersion: form?.consent_policy_version ?? "",
    redirectUrl: form?.redirect_url ?? "",
    successMessage: form?.success_message ?? "",
  };
}

const optional = (text: string) => (text.trim() === "" ? null : text.trim());

function originsOf(draft: WebFormDraft): string[] {
  return draft.origins
    .split(/[\s,]+/)
    .map((origin) => origin.trim())
    .filter(Boolean);
}

/**
 * Every required field the draft leaves empty, with the sentence it owes.
 *
 * The server refuses only the first empty field it meets. A form that waited
 * for it would make the reader press Create once per missing field.
 */
export function missingFields(
  draft: WebFormDraft,
): ReadonlyMap<string, MessageKey> {
  const missing = new Map<string, MessageKey>();
  if (draft.name.trim() === "") {
    missing.set("name", "webForms.required.name");
  }
  if (draft.sender === "browser" && originsOf(draft).length === 0) {
    missing.set("allowed_origins", "webForms.required.origins");
  }
  if (draft.consentRequired && draft.consentWording.trim() === "") {
    missing.set("consent_wording", "webForms.required.consentWording");
  }
  return missing;
}

// The website an entry names, as a post's Origin header spells it: scheme, host
// and a port only when it is not the default. Null for text that is no address.
function websiteOf(entry: string): string | null {
  if (!URL.canParse(entry)) {
    return null;
  }
  const { origin } = new URL(entry);
  return origin === "null" ? null : origin;
}

/**
 * The entries saved as some other spelling of their website, such as a page
 * address or a default port. A path cannot narrow who may post, so the reader
 * is told before saving.
 */
export function narrowedOrigins(
  draft: WebFormDraft,
): { entry: string; origin: string }[] {
  return originsOf(draft).flatMap((entry) => {
    const origin = websiteOf(entry);
    const same = origin === entry || `${origin}/` === entry;
    return origin !== null && !same ? [{ entry, origin }] : [];
  });
}

// The settings create and update share. Webflow's secret is sent only when one
// was typed, so an edit that leaves the field empty keeps the stored one.
export function webFormBodyOf(draft: WebFormDraft) {
  // Sent as the Origin header spells it, so a stored `:443` never refuses the
  // site. Text that is no address goes as typed, for the server to refuse.
  const origins = originsOf(draft).map((entry) => websiteOf(entry) ?? entry);
  return {
    name: draft.name.trim(),
    ...(draft.sender === "browser" ? { allowed_origins: origins } : {}),
    ...(pastesSecret(draft.sender) && draft.senderSecret.trim() !== ""
      ? { sender_secret: draft.senderSecret.trim() }
      : {}),
    consent_required: draft.consentRequired,
    consent_wording: optional(draft.consentWording),
    consent_policy_version: optional(draft.consentPolicyVersion),
    redirect_url: optional(draft.redirectUrl),
    success_message: optional(draft.successMessage),
  };
}

type SaveVariables = Readonly<{ draft: WebFormDraft; form?: WebForm }>;

async function saveWebForm({
  draft,
  form,
}: SaveVariables): Promise<WebForm | WebFormCreated> {
  if (form) {
    return unwrap(
      await api.PATCH("/web-forms/{id}", {
        params: { path: { id: form.id }, ...ifMatch(form.version) },
        body: webFormBodyOf(draft),
      }),
    );
  }
  return unwrap(
    await api.POST("/web-forms", {
      body: { sender: draft.sender, ...webFormBodyOf(draft) },
    }),
  );
}

const FIELDS_WITH_A_SLOT = new Set([
  "name",
  "sender",
  "allowed_origins",
  "sender_secret",
  "consent_wording",
  "consent_policy_version",
  "redirect_url",
  "success_message",
]);

// Each field's sentence, and the one line for a refusal no field can hold. A
// missing field speaks over the server's word for it, which is a fragment.
function useDrawerErrors(
  failure: Error | null,
  missing: ReadonlyMap<string, MessageKey>,
) {
  const t = useT();
  const fieldErrors = new Map(
    problemFieldErrorsOf(failure).map((p) => [p.field, p.message]),
  );
  for (const [field, key] of missing) {
    fieldErrors.set(field, t(key));
  }
  const unplaced = [...fieldErrors.entries()].filter(
    ([field]) => !FIELDS_WITH_A_SLOT.has(field),
  );
  let generalError: string | null = null;
  if (failure) {
    generalError =
      fieldErrors.size === 0
        ? problemMessageOf(failure, t)
        : (unplaced[0]?.[1] ?? null);
  }
  return { fieldErrors, generalError };
}

// Where the form lives: fixed once it exists, so an edit only names it.
function SenderField({
  sender,
  editing,
  senders,
  error,
  onChange,
}: Readonly<{
  sender: WebFormSender;
  editing: boolean;
  senders: readonly WebFormSender[];
  error?: string;
  onChange: (sender: WebFormSender) => void;
}>) {
  const t = useT();
  if (editing) {
    return (
      <Field
        label={t("webForms.field.sender")}
        hint={t("webForms.field.senderFixed")}
      >
        {(field) => (
          <TextInput
            {...field}
            value={t(`webForms.sender.${sender}`)}
            disabled
          />
        )}
      </Field>
    );
  }
  return (
    <>
      <ChoiceList
        legend={t("webForms.field.sender")}
        layout="cards"
        value={sender}
        choices={senders.map((choice) => ({
          value: choice,
          label: t(`webForms.sender.${choice}`),
          description: t(`webForms.senderHint.${choice}`),
        }))}
        onChange={onChange}
      />
      {error && <ErrorLine>{error}</ErrorLine>}
    </>
  );
}

export function WebFormDrawer({
  form,
  secretsAvailable = true,
  onClose,
  onCreated,
}: Readonly<{
  form?: WebForm;
  secretsAvailable?: boolean;
  onClose: () => void;
  onCreated?: (created: WebFormCreated) => void;
}>) {
  const t = useT();
  const titleId = useId();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<WebFormDraft>(() => draftFor(form));
  // Set by the first press of Create, so an empty field is not flagged before
  // the reader has had a chance to fill it.
  const [checked, setChecked] = useState(false);
  const save = useMutation({
    mutationFn: saveWebForm,
    onSuccess: (saved, { form: edited }) => {
      queryClient.invalidateQueries({ queryKey: [WEB_FORMS_KEY] });
      if (!edited) {
        onCreated?.(saved);
      }
      onClose();
    },
  });
  // A corrected field should not keep the refusal it no longer earns.
  const set = (patch: Partial<WebFormDraft>) => {
    if (save.isError) {
      save.reset();
    }
    setDraft((prev) => ({ ...prev, ...patch }));
  };
  const editing = form !== undefined;
  const missing = missingFields(draft);
  const { fieldErrors, generalError } = useDrawerErrors(
    save.error,
    checked ? missing : new Map(),
  );
  const narrowed = draft.sender === "browser" ? narrowedOrigins(draft) : [];
  // Every sender but a browser verifies against a stored secret, so an
  // installation without a secret store offers the browser alone.
  const senders = secretsAvailable ? WEB_FORM_SENDERS : ["browser" as const];
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer-reading"
      closeDisabled={save.isPending}
    >
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {editing ? t("webForms.drawer.editTitle") : t("webForms.new")}
        </Heading>
        <p>{t("webForms.drawer.sub")}</p>
      </DrawerHead>
      <DrawerBody className="form-stack">
        <SenderField
          sender={draft.sender}
          editing={editing}
          senders={senders}
          error={fieldErrors.get("sender")}
          onChange={(sender) => set({ sender })}
        />
        <Field
          label={t("webForms.field.name")}
          required
          hint={t("webForms.field.nameHint")}
          error={fieldErrors.get("name")}
        >
          {(field) => (
            <TextInput
              {...field}
              value={draft.name}
              onChange={(event) => set({ name: event.target.value })}
            />
          )}
        </Field>
        {draft.sender === "browser" && (
          <Field
            label={t("webForms.field.origins")}
            required
            hint={t("webForms.field.originsHint")}
            error={fieldErrors.get("allowed_origins")}
          >
            {(field) => (
              <Textarea
                {...field}
                rows={3}
                value={draft.origins}
                placeholder={t("webForms.field.originsPlaceholder")}
                onChange={(event) => set({ origins: event.target.value })}
              />
            )}
          </Field>
        )}
        {narrowed.map(({ entry, origin }) => (
          <p key={entry} className="t-caption">
            {t("webForms.field.originKept", { entry, origin })}
          </p>
        ))}
        {pastesSecret(draft.sender) && (
          <Field
            label={t("webForms.field.senderSecret")}
            hint={
              editing && form.secret_configured
                ? t("webForms.field.senderSecretKeep")
                : t("webForms.field.senderSecretHint")
            }
            error={fieldErrors.get("sender_secret")}
          >
            {(field) => (
              <TextInput
                {...field}
                type="password"
                autoComplete="off"
                value={draft.senderSecret}
                onChange={(event) => set({ senderSecret: event.target.value })}
              />
            )}
          </Field>
        )}
        <Checkbox
          checked={draft.consentRequired}
          label={t("webForms.field.consentRequired")}
          onChange={(event) => set({ consentRequired: event.target.checked })}
        />
        {draft.consentRequired && (
          <>
            <Field
              label={t("webForms.field.consentWording")}
              required
              hint={t("webForms.field.consentWordingHint")}
              error={fieldErrors.get("consent_wording")}
            >
              {(field) => (
                <Textarea
                  {...field}
                  rows={3}
                  value={draft.consentWording}
                  onChange={(event) =>
                    set({ consentWording: event.target.value })
                  }
                />
              )}
            </Field>
            <Field
              label={t("webForms.field.consentPolicyVersion")}
              hint={t("webForms.field.consentPolicyVersionHint")}
              error={fieldErrors.get("consent_policy_version")}
            >
              {(field) => (
                <TextInput
                  {...field}
                  value={draft.consentPolicyVersion}
                  onChange={(event) =>
                    set({ consentPolicyVersion: event.target.value })
                  }
                />
              )}
            </Field>
          </>
        )}
        {draft.sender === "browser" && (
          <Field
            label={t("webForms.field.redirectUrl")}
            hint={t("webForms.field.redirectUrlHint")}
            error={fieldErrors.get("redirect_url")}
          >
            {(field) => (
              <TextInput
                {...field}
                type="url"
                value={draft.redirectUrl}
                placeholder={t("webForms.field.redirectUrlPlaceholder")}
                onChange={(event) => set({ redirectUrl: event.target.value })}
              />
            )}
          </Field>
        )}
        <Field
          label={t("webForms.field.successMessage")}
          hint={t("webForms.field.successMessageHint")}
          error={fieldErrors.get("success_message")}
        >
          {(field) => (
            <TextInput
              {...field}
              value={draft.successMessage}
              onChange={(event) => set({ successMessage: event.target.value })}
            />
          )}
        </Field>
        {generalError && <ErrorLine>{generalError}</ErrorLine>}
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button onClick={onClose} disabled={save.isPending}>
          {t("webForms.drawer.cancel")}
        </Button>
        <Button
          variant="primary"
          data-testid="save-web-form"
          pending={save.isPending}
          onClick={() => {
            setChecked(true);
            if (missing.size === 0) {
              save.mutate({ draft, form });
            }
          }}
        >
          {editing ? t("webForms.drawer.save") : t("webForms.drawer.create")}
        </Button>
      </DrawerFoot>
    </Modal>
  );
}
