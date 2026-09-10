// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one editor for a person's emails and numbers. The wire is a WHOLE-LIST
// replace (UpdatePersonRequest.emails/.phones carry no per-row id), so the
// modal stages the full set and Save sends it once, pinned to the person's
// version. A per-row save would re-send the whole array on every keystroke
// commit and widen the version-skew window for nothing.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronUp, Trash2 } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { Button, Card, Modal, Radio, TextInput } from "../design-system/atoms";
import { Select, type SelectOption } from "../design-system/select";
import { Row, Stack } from "../design-system/stack";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import {
  blankEmail,
  blankPhone,
  emailTypeOptions,
  isEmailType,
  isPhoneType,
  moveRow,
  phoneTypeOptions,
  removeRow,
  replaceRow,
  type StagedEmail,
  type StagedPhone,
  seedEmails,
  seedPhones,
  selectPrimary,
  toEmailInputs,
  toPhoneInputs,
} from "./personcontactedit.helpers";

type Person = components["schemas"]["Person"];
type PersonEmailInput = components["schemas"]["PersonEmailInput"];
type PersonPhoneInput = components["schemas"]["PersonPhoneInput"];

// --- the write ---------------------------------------------------------

type ContactMethodsPatch = Readonly<{
  person: Person;
  emails: readonly PersonEmailInput[];
  phones: readonly PersonPhoneInput[];
}>;

async function patchContactMethods({
  person,
  emails,
  phones,
}: ContactMethodsPatch): Promise<void> {
  const { error } = await api.PATCH("/people/{id}", {
    params: {
      path: { id: person.id },
      ...ifMatch(requireVersion(person.version)),
    },
    body: { emails: [...emails], phones: [...phones] },
  });
  if (error) {
    throwProblem(error);
  }
}

// The same invalidation `usePersonFieldPatch` (personrail.tsx) makes after any
// person write: person360 is what the rail and the rest of the record draw
// the contact's emails and phones from, and personBrief comes with it because
// the brief's own sentences can name an address.
function useContactMethodsSave() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: patchContactMethods,
    onSuccess: async (_result, { person }) => {
      await queryClient.invalidateQueries({
        queryKey: ["person360", person.id],
      });
      await queryClient.invalidateQueries({
        queryKey: ["personBrief", person.id],
      });
    },
  });
}

// --- one row ----------------------------------------------------------

// The one row shape emails and phones both are: a type, a value, a primary
// radio, reorder verbs and a remove verb. Two copies of this — one keyed to
// `email`/`email_type`, one to `phone`/`phone_type` — differed in nothing but
// which field they read, so the caller now supplies that binding instead of
// the row shape being duplicated.
type ContactRowProps = Readonly<{
  disabled: boolean;
  typeOptions: readonly SelectOption[];
  typeValue: string;
  onTypeChange: (value: string) => void;
  valueLabel: string;
  value: string;
  onValueChange: (value: string) => void;
  isPrimary: boolean;
  primaryGroup: string;
  onPrimary: () => void;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onMoveUp: () => void;
  onMoveDown: () => void;
  moveUpLabel: string;
  moveDownLabel: string;
  removeLabel: string;
  onRemove: () => void;
}>;

function ContactRowEditor({
  disabled,
  typeOptions,
  typeValue,
  onTypeChange,
  valueLabel,
  value,
  onValueChange,
  isPrimary,
  primaryGroup,
  onPrimary,
  canMoveUp,
  canMoveDown,
  onMoveUp,
  onMoveDown,
  moveUpLabel,
  moveDownLabel,
  removeLabel,
  onRemove,
}: ContactRowProps) {
  const t = useT();
  return (
    <Card as="div">
      <Row gap="2" align="center">
        <Select
          options={typeOptions}
          value={typeValue}
          onChange={onTypeChange}
          aria-label={t("person.rail.contactType")}
          disabled={disabled}
        />
        <TextInput
          aria-label={valueLabel}
          value={value}
          disabled={disabled}
          style={{ flex: 1, minWidth: "16ch" }}
          onChange={(event) => onValueChange(event.target.value)}
        />
        <Radio
          name={primaryGroup}
          label={t("person.rail.contactPrimary")}
          checked={isPrimary}
          disabled={disabled}
          onChange={onPrimary}
        />
        <Button
          small
          variant="ghost"
          disabled={disabled || !canMoveUp}
          aria-label={moveUpLabel}
          onClick={onMoveUp}
        >
          <ChevronUp aria-hidden size={16} />
        </Button>
        <Button
          small
          variant="ghost"
          disabled={disabled || !canMoveDown}
          aria-label={moveDownLabel}
          onClick={onMoveDown}
        >
          <ChevronDown aria-hidden size={16} />
        </Button>
        <Button
          small
          variant="ghost"
          disabled={disabled}
          aria-label={removeLabel}
          onClick={onRemove}
        >
          <Trash2 aria-hidden size={16} />
        </Button>
      </Row>
    </Card>
  );
}

// --- the modal -------------------------------------------------------------

export function EditContactMethodsModal({
  open,
  onClose,
  person,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  person: components["schemas"]["Person"];
}>) {
  const t = useT();
  const headingId = useId();
  const [emails, setEmails] = useState<StagedEmail[]>(() => seedEmails(person));
  const [phones, setPhones] = useState<StagedPhone[]>(() => seedPhones(person));
  const save = useContactMethodsSave();

  // The latest `person`, read by the reseed effect below WITHOUT being one of
  // its dependencies — written in its own effect rather than during render,
  // because React may discard a render and a ref assigned in one would
  // publish a value that was never committed (the same rule adddocument.tsx's
  // own `openNow` ref keeps).
  const personRef = useRef(person);
  useEffect(() => {
    personRef.current = person;
  }, [person]);

  // Re-stages every row the moment the modal OPENS, and only then: the modal
  // stays mounted between opens rather than remounting, so a plain effect on
  // `person` would re-seed on every background person360 refetch too — wiping
  // out whatever the reader was mid-way through typing.
  useEffect(() => {
    if (open) {
      setEmails(seedEmails(personRef.current));
      setPhones(seedPhones(personRef.current));
    }
  }, [open]);

  function close() {
    save.reset();
    onClose();
  }

  function handleSave() {
    save.mutate(
      {
        person,
        emails: toEmailInputs(emails),
        phones: toPhoneInputs(phones),
      },
      { onSuccess: close },
    );
  }

  return (
    <Modal open={open} onClose={close} labelledBy={headingId}>
      <h2
        id={headingId}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {t("person.rail.editContactMethods")}
      </h2>
      <Stack gap="4">
        <Stack gap="2">
          {emails.map((row, index) => (
            <ContactRowEditor
              key={row.key}
              disabled={save.isPending}
              typeOptions={emailTypeOptions(t)}
              typeValue={row.email_type}
              onTypeChange={(value) => {
                if (isEmailType(value)) {
                  setEmails((rows) =>
                    replaceRow(rows, row.key, { email_type: value }),
                  );
                }
              }}
              valueLabel={t("person.rail.contactValueEmail")}
              value={row.email}
              onValueChange={(next) =>
                setEmails((rows) => replaceRow(rows, row.key, { email: next }))
              }
              isPrimary={row.is_primary}
              primaryGroup={`email-primary-${row.email_type}`}
              onPrimary={() =>
                setEmails((rows) =>
                  selectPrimary(rows, row.key, (r) => r.email_type),
                )
              }
              canMoveUp={index > 0}
              canMoveDown={index < emails.length - 1}
              onMoveUp={() => setEmails((rows) => moveRow(rows, row.key, "up"))}
              onMoveDown={() =>
                setEmails((rows) => moveRow(rows, row.key, "down"))
              }
              moveUpLabel={`${t("person.rail.contactMoveUp")} ${row.email || t("person.rail.contactValueEmail")}`}
              moveDownLabel={`${t("person.rail.contactMoveDown")} ${row.email || t("person.rail.contactValueEmail")}`}
              removeLabel={`${t("person.rail.contactRemove")} ${row.email || t("person.rail.contactValueEmail")}`}
              onRemove={() => setEmails((rows) => removeRow(rows, row.key))}
            />
          ))}
          <Button
            small
            variant="ghost"
            disabled={save.isPending}
            onClick={() =>
              setEmails((rows) => [...rows, blankEmail(rows.length)])
            }
          >
            {t("person.rail.addEmail")}
          </Button>
        </Stack>
        <Stack gap="2">
          {phones.map((row, index) => (
            <ContactRowEditor
              key={row.key}
              disabled={save.isPending}
              typeOptions={phoneTypeOptions(t)}
              typeValue={row.phone_type}
              onTypeChange={(value) => {
                if (isPhoneType(value)) {
                  setPhones((rows) =>
                    replaceRow(rows, row.key, { phone_type: value }),
                  );
                }
              }}
              valueLabel={t("person.rail.contactValuePhone")}
              value={row.phone}
              onValueChange={(next) =>
                setPhones((rows) => replaceRow(rows, row.key, { phone: next }))
              }
              isPrimary={row.is_primary}
              primaryGroup={`phone-primary-${row.phone_type}`}
              onPrimary={() =>
                setPhones((rows) =>
                  selectPrimary(rows, row.key, (r) => r.phone_type),
                )
              }
              canMoveUp={index > 0}
              canMoveDown={index < phones.length - 1}
              onMoveUp={() => setPhones((rows) => moveRow(rows, row.key, "up"))}
              onMoveDown={() =>
                setPhones((rows) => moveRow(rows, row.key, "down"))
              }
              moveUpLabel={`${t("person.rail.contactMoveUp")} ${row.phone || t("person.rail.contactValuePhone")}`}
              moveDownLabel={`${t("person.rail.contactMoveDown")} ${row.phone || t("person.rail.contactValuePhone")}`}
              removeLabel={`${t("person.rail.contactRemove")} ${row.phone || t("person.rail.contactValuePhone")}`}
              onRemove={() => setPhones((rows) => removeRow(rows, row.key))}
            />
          ))}
          <Button
            small
            variant="ghost"
            disabled={save.isPending}
            onClick={() =>
              setPhones((rows) => [...rows, blankPhone(rows.length)])
            }
          >
            {t("person.rail.addPhone")}
          </Button>
        </Stack>
      </Stack>
      {save.isError && (
        <p
          className="t-caption"
          role="alert"
          style={{ color: "var(--danger)" }}
        >
          {problemMessageOf(save.error, t)}
        </p>
      )}
      <div className="actions">
        <Button onClick={close} disabled={save.isPending}>
          {t("create.cancel")}
        </Button>
        <Button
          variant="primary"
          disabled={save.isPending}
          onClick={handleSave}
        >
          {t("record.save")}
        </Button>
      </div>
    </Modal>
  );
}
