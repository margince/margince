// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type FormEvent, useId, useState } from "react";
import {
  Button,
  Field,
  SectionHeader,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { RecordFormDialog } from "../../design-system/recordformdialog";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { countryOptions } from "../../format/assignedcountries";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import {
  type LegalEntity,
  type UpdateLegalEntity,
  useArchiveLegalEntity,
  useLegalEntityLogo,
  useUpdateLegalEntity,
} from "../../lib/legalentities.queries";
import { problemMessageOf } from "../../lib/problem";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";
import {
  DefaultEntityJurisdiction,
  jurisdictionOptions,
  useInstallationJurisdiction,
} from "./jurisdictionentities";
import {
  ADDRESS_KEYS,
  DEFAULT_ENTITY_KEYS,
  type Draft,
  type DraftKey,
  ENTITY_KEYS,
  entityLabels,
  entityRefusal,
  FLAT_KEYS,
  LABEL_KEYS,
  missingEntityFields,
  OFFERING_KEYS,
  wireOf,
} from "./legalentityfields";
import { EntityLogo } from "./legalentitylogo";

const orEmpty = (value: string | null | undefined) => value ?? "";

function draftOf(entity: LegalEntity): Draft {
  const address = entity.address;
  const offering = entity.offering ?? {};
  return {
    display_name: entity.display_name,
    legal_name: orEmpty(entity.legal_name),
    line1: orEmpty(address.line1),
    line2: orEmpty(address.line2),
    postal_code: orEmpty(address.postal_code),
    city: orEmpty(address.city),
    region: orEmpty(address.region),
    country: orEmpty(entity.country),
    jurisdiction: orEmpty(entity.jurisdiction),
    vat_id: orEmpty(entity.vat_id),
    register_court: orEmpty(entity.register_court),
    register_number: orEmpty(entity.register_number),
    default_language: orEmpty(entity.default_language),
    bank_account_holder: orEmpty(entity.bank_account_holder),
    iban: orEmpty(entity.iban),
    bic: orEmpty(entity.bic),
    imprint: orEmpty(entity.imprint),
    signature_template: orEmpty(entity.signature_template),
    offer_summary: orEmpty(offering.offer_summary),
    icp: orEmpty(offering.icp),
    value_proposition: orEmpty(offering.value_proposition),
    usp: orEmpty(offering.usp),
  };
}

// Only what the reader changed is sent: the server keeps a field left out, and
// clears one sent empty. So a save never touches a field the reader did not.
function changesOf(before: Draft, after: Draft): UpdateLegalEntity {
  const changed = (key: DraftKey) => before[key] !== after[key];
  const pick = (keys: readonly DraftKey[]) =>
    Object.fromEntries(
      keys.filter(changed).map((key) => [key, after[key].trim()]),
    );
  const body: UpdateLegalEntity = pick(FLAT_KEYS);
  if (ADDRESS_KEYS.some(changed)) body.address = pick(ADDRESS_KEYS);
  if (OFFERING_KEYS.some(changed)) body.offering = pick(OFFERING_KEYS);
  return body;
}

/**
 * Everything an entity says about itself, in one form. The default entity
 * reads its name, address, register entry, offering and logo from the company
 * profile. So the form offers it only the fields it keeps itself.
 */
export function EditEntityDialog({
  entity,
  onClose,
}: Readonly<{ entity: LegalEntity; onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const formId = useId();
  const update = useUpdateLegalEntity();
  const logo = useLegalEntityLogo();
  // Both writes are conditional on one version, so neither starts while the
  // other is out, and the dialog stays open until the logo write answers.
  const busy = update.isPending || logo.isPending;
  const [initial] = useState(() => draftOf(entity));
  const [draft, setDraft] = useState(initial);
  // A logo write moves the version on, and the next save must name it.
  const [version, setVersion] = useState(entity.version);
  const own = !entity.is_default;
  const body = changesOf(initial, draft);
  const dirty = Object.keys(body).length > 0;
  const labels = entityLabels(t, own ? ENTITY_KEYS : DEFAULT_ENTITY_KEYS);
  const [faults, faultsRoot] = useFieldFaults();
  const refused = entityRefusal(update.error, t, labels);
  const text = (key: DraftKey) => ({
    label: t(LABEL_KEYS[key]),
    value: draft[key],
    error: faults.of(wireOf(key)),
    onChange: (value: string) => {
      setDraft((current) => ({ ...current, [key]: value }));
      faults.edited(wireOf(key));
    },
  });
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (busy) return;
    if (!dirty) {
      onClose();
      return;
    }
    // Only a field the save sends can be refused; the server keeps the rest.
    const blanked = [...missingEntityFields(draft, labels, locale)].filter(
      ([key]) => Object.hasOwn(body, key),
    );
    if (own && faults.show(new Map(blanked))) {
      update.reset();
      return;
    }
    update.mutate(
      { id: entity.id, version, body },
      {
        onSuccess: onClose,
        onError: (error) =>
          faults.show(entityRefusal(error, t, labels).byField),
      },
    );
  };
  return (
    <RecordFormDialog
      open
      title={t("legalEntities.editTitle", {
        name: entity.display_name || t("legalEntities.defaultUnnamed"),
      })}
      onClose={onClose}
      fieldCount={own ? 22 : 6}
      unsaved={dirty}
      closeDisabled={busy}
      form={
        <form
          id={formId}
          ref={faultsRoot}
          onSubmit={submit}
          className="form-stack"
          noValidate
        >
          {own ? (
            <>
              <TextField
                maxLength={TEXT_LIMITS.label}
                required
                {...text("display_name")}
              />
              <TextField
                maxLength={TEXT_LIMITS.line}
                required
                {...text("legal_name")}
              />
            </>
          ) : (
            <p className="t-caption">{t("legalEntities.defaultFromCompany")}</p>
          )}
          {own ? (
            <>
              <CountryField {...text("country")} />
              <JurisdictionField {...text("jurisdiction")} />
            </>
          ) : (
            <DefaultEntityJurisdiction />
          )}
          {own && (
            <>
              <SectionHeader
                level={3}
                title={t("legalEntities.sectionAddress")}
              />
              <TextField {...text("line1")} />
              <TextField {...text("line2")} />
              <TextField {...text("postal_code")} />
              <TextField {...text("city")} />
              <TextField {...text("region")} />
              <SectionHeader
                level={3}
                title={t("legalEntities.sectionRegister")}
              />
              <TextField {...text("vat_id")} />
              <TextField {...text("register_court")} />
              <TextField {...text("register_number")} />
            </>
          )}
          <SectionHeader level={3} title={t("legalEntities.sectionBank")} />
          <TextField {...text("bank_account_holder")} />
          <TextField {...text("iban")} />
          <TextField {...text("bic")} />
          <SectionHeader
            level={3}
            title={t("legalEntities.sectionDocuments")}
          />
          <TextField
            hint={t("legalEntities.defaultLanguageHint")}
            maxLength={2}
            {...text("default_language")}
          />
          <AreaField
            hint={t("legalEntities.imprintHint", {
              max: formatNumber(TEXT_LIMITS.document, locale),
            })}
            {...text("imprint")}
          />
          {/* The default entity signs with the workspace template, which has
              its own card under Mail. */}
          {own && (
            <AreaField
              hint={t("legalEntities.signatureTemplateHint")}
              {...text("signature_template")}
            />
          )}
          {own && (
            <>
              <SectionHeader
                level={3}
                title={t("legalEntities.sectionOffering")}
              />
              <AreaField {...text("offer_summary")} />
              <AreaField {...text("icp")} />
              <AreaField {...text("value_proposition")} />
              <AreaField {...text("usp")} />
              <EntityLogo
                entity={entity}
                write={logo}
                saving={update.isPending}
                version={version}
                onVersion={setVersion}
              />
            </>
          )}
          {refused.rest !== null && <ErrorLine>{refused.rest}</ErrorLine>}
        </form>
      }
      actions={
        <>
          <Button disabled={busy} onClick={onClose}>
            {t("deals.cancel")}
          </Button>
          <Button
            type="submit"
            form={formId}
            variant="primary"
            disabled={logo.isPending}
            pending={update.isPending}
          >
            {t("legalEntities.save")}
          </Button>
        </>
      }
    />
  );
}

export function CountryField({
  value,
  error,
  onChange,
}: Readonly<{
  value: string;
  error?: string;
  onChange: (value: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // The contract speaks lower case; the region table upper case.
  const options = countryOptions(locale).map((option) => ({
    value: option.value.toLowerCase(),
    label: option.label,
  }));
  return (
    <Field
      label={t("legalEntities.country")}
      hint={t("legalEntities.countryHint")}
      required
      error={error}
    >
      {(control) => (
        <Select
          {...control}
          value={value}
          placeholder={t("legalEntities.countryPlaceholder")}
          options={options}
          onChange={onChange}
        />
      )}
    </Field>
  );
}

/** The law a further entity follows: one of the packs this build offers. */
export function JurisdictionField({
  value,
  error,
  onChange,
}: Readonly<{
  value: string;
  error?: string;
  onChange: (value: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const query = useInstallationJurisdiction();
  return (
    <Field
      label={t("legalEntities.jurisdiction")}
      hint={t("legalEntities.jurisdictionHint")}
      required
      error={error}
    >
      {(control) => (
        <Select
          {...control}
          value={value}
          placeholder={t("jurisdiction.change.placeholder")}
          options={jurisdictionOptions(query.data?.available ?? [], locale)}
          onChange={onChange}
        />
      )}
    </Field>
  );
}

type TextProps = Readonly<{
  label: string;
  hint?: string;
  error?: string;
  required?: boolean;
  maxLength?: number;
  value: string;
  onChange: (value: string) => void;
}>;

export function TextField({
  label,
  hint,
  error,
  required,
  maxLength,
  value,
  onChange,
}: TextProps) {
  return (
    <Field label={label} hint={hint} error={error} required={required}>
      {(control) => (
        <TextInput
          {...control}
          required={required}
          maxLength={maxLength}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </Field>
  );
}

function AreaField({ label, hint, error, value, onChange }: TextProps) {
  return (
    <Field label={label} hint={hint} error={error}>
      {(control) => (
        <Textarea
          {...control}
          rows={3}
          maxLength={TEXT_LIMITS.document}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </Field>
  );
}

export function ArchiveEntityDialog({
  entity,
  onClose,
}: Readonly<{ entity: LegalEntity | null; onClose: () => void }>) {
  const t = useT();
  const archive = useArchiveLegalEntity();
  return (
    <ConfirmModal
      open={entity !== null}
      onClose={() => {
        archive.reset();
        onClose();
      }}
      title={t("legalEntities.archiveTitle", {
        name: entity?.display_name ?? "",
      })}
      confirmLabel={t("legalEntities.archive")}
      confirmVariant="danger"
      pending={archive.isPending}
      error={archive.isError ? problemMessageOf(archive.error, t) : null}
      onConfirm={() => {
        if (entity) {
          archive.mutate(
            { id: entity.id, version: entity.version },
            { onSuccess: onClose },
          );
        }
      }}
    >
      <p>{t("legalEntities.archiveBody")}</p>
    </ConfirmModal>
  );
}
