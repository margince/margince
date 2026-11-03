// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type FormEvent, useId, useState } from "react";
import {
  Avatar,
  Button,
  Field,
  SectionHeader,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { CompanyLogo } from "../../design-system/companylogo";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { FileDropzone } from "../../design-system/filedropzone";
import { RecordFormDialog } from "../../design-system/recordformdialog";
import { Select } from "../../design-system/select";
import { countryOptions } from "../../format/assignedcountries";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import {
  type LegalEntity,
  type UpdateLegalEntity,
  useArchiveLegalEntity,
  useLegalEntityLogo,
  useUpdateLegalEntity,
} from "./legalentities.queries";

// The editor's fields, flat. The contract nests the address and the offering;
// draftOf and changesOf are the one place that maps between the two shapes.
type Draft = Readonly<{
  display_name: string;
  legal_name: string;
  line1: string;
  line2: string;
  postal_code: string;
  city: string;
  region: string;
  country: string;
  vat_id: string;
  register_court: string;
  register_number: string;
  default_language: string;
  bank_account_holder: string;
  iban: string;
  bic: string;
  imprint: string;
  offer_summary: string;
  icp: string;
  value_proposition: string;
  usp: string;
}>;
type DraftKey = keyof Draft;

const ADDRESS_KEYS = [
  "line1",
  "line2",
  "postal_code",
  "city",
  "region",
] as const satisfies readonly DraftKey[];
const OFFERING_KEYS = [
  "offer_summary",
  "icp",
  "value_proposition",
  "usp",
] as const satisfies readonly DraftKey[];
const FLAT_KEYS = [
  "display_name",
  "legal_name",
  "country",
  "vat_id",
  "register_court",
  "register_number",
  "default_language",
  "bank_account_holder",
  "iban",
  "bic",
  "imprint",
] as const satisfies readonly DraftKey[];

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
    vat_id: orEmpty(entity.vat_id),
    register_court: orEmpty(entity.register_court),
    register_number: orEmpty(entity.register_number),
    default_language: orEmpty(entity.default_language),
    bank_account_holder: orEmpty(entity.bank_account_holder),
    iban: orEmpty(entity.iban),
    bic: orEmpty(entity.bic),
    imprint: orEmpty(entity.imprint),
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
  const pick = <K extends DraftKey>(keys: readonly K[]) =>
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
  const formId = useId();
  const update = useUpdateLegalEntity();
  const [initial] = useState(() => draftOf(entity));
  const [draft, setDraft] = useState(initial);
  // A logo write moves the version on, and the next save must name it.
  const [version, setVersion] = useState(entity.version);
  const own = !entity.is_default;
  const body = changesOf(initial, draft);
  const dirty = Object.keys(body).length > 0;
  const text = (key: DraftKey) => ({
    value: draft[key],
    onChange: (value: string) =>
      setDraft((current) => ({ ...current, [key]: value })),
  });
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!dirty) {
      onClose();
      return;
    }
    update.mutate({ id: entity.id, version, body }, { onSuccess: onClose });
  };
  return (
    <RecordFormDialog
      open
      title={t("legalEntities.editTitle", {
        name: entity.display_name || t("legalEntities.defaultUnnamed"),
      })}
      onClose={onClose}
      fieldCount={own ? 20 : 7}
      unsaved={dirty}
      closeDisabled={update.isPending}
      form={
        <form id={formId} onSubmit={submit} className="form-stack">
          {own ? (
            <>
              <TextField
                label={t("legalEntities.displayName")}
                required
                {...text("display_name")}
              />
              <TextField
                label={t("legalEntities.legalName")}
                required
                {...text("legal_name")}
              />
            </>
          ) : (
            <p className="t-caption">{t("legalEntities.defaultFromCompany")}</p>
          )}
          <CountryField
            required={own}
            value={draft.country}
            onChange={text("country").onChange}
          />
          {own && (
            <>
              <SectionHeader
                level={3}
                title={t("legalEntities.sectionAddress")}
              />
              <TextField label={t("legalEntities.line1")} {...text("line1")} />
              <TextField label={t("legalEntities.line2")} {...text("line2")} />
              <TextField
                label={t("legalEntities.postalCode")}
                {...text("postal_code")}
              />
              <TextField label={t("legalEntities.city")} {...text("city")} />
              <TextField
                label={t("legalEntities.region")}
                {...text("region")}
              />
              <SectionHeader
                level={3}
                title={t("legalEntities.sectionRegister")}
              />
              <TextField label={t("legalEntities.vatId")} {...text("vat_id")} />
              <TextField
                label={t("legalEntities.registerCourt")}
                {...text("register_court")}
              />
              <TextField
                label={t("legalEntities.registerNumber")}
                {...text("register_number")}
              />
            </>
          )}
          <SectionHeader level={3} title={t("legalEntities.sectionBank")} />
          <TextField
            label={t("legalEntities.bankHolder")}
            {...text("bank_account_holder")}
          />
          <TextField label={t("legalEntities.iban")} {...text("iban")} />
          <TextField label={t("legalEntities.bic")} {...text("bic")} />
          <SectionHeader
            level={3}
            title={t("legalEntities.sectionDocuments")}
          />
          <TextField
            label={t("legalEntities.defaultLanguage")}
            hint={t("legalEntities.defaultLanguageHint")}
            maxLength={2}
            {...text("default_language")}
          />
          <AreaField
            label={t("legalEntities.imprint")}
            hint={t("legalEntities.imprintHint")}
            {...text("imprint")}
          />
          {own && (
            <>
              <SectionHeader
                level={3}
                title={t("legalEntities.sectionOffering")}
              />
              <AreaField
                label={t("legalEntities.offerSummary")}
                {...text("offer_summary")}
              />
              <AreaField label={t("legalEntities.icp")} {...text("icp")} />
              <AreaField
                label={t("legalEntities.valueProposition")}
                {...text("value_proposition")}
              />
              <AreaField label={t("legalEntities.usp")} {...text("usp")} />
              <EntityLogo
                entity={entity}
                version={version}
                onVersion={setVersion}
              />
            </>
          )}
          <ErrorLine
            error={update.isError && problemMessageOf(update.error, t)}
          />
        </form>
      }
      actions={
        <>
          <Button disabled={update.isPending} onClick={onClose}>
            {t("deals.cancel")}
          </Button>
          <Button
            type="submit"
            form={formId}
            variant="primary"
            pending={update.isPending}
          >
            {t("legalEntities.save")}
          </Button>
        </>
      }
    />
  );
}

const ACCEPTED_IMAGES =
  "image/png,image/jpeg,image/gif,image/webp,image/svg+xml,image/x-icon";

// The logo is stored the moment it is chosen, as on the company profile. A
// file is no text the form could hold back until Save.
function EntityLogo({
  entity,
  version,
  onVersion,
}: Readonly<{
  entity: LegalEntity;
  version: number;
  onVersion: (version: number) => void;
}>) {
  const t = useT();
  const write = useLegalEntityLogo();
  const [src, setSrc] = useState(entity.logo_url ?? null);
  const send = (file: File | null) =>
    write.mutate(
      { id: entity.id, version, file },
      {
        onSuccess: (written) => {
          if (written === null) return;
          onVersion(written.version);
          setSrc(written.logoUrl);
        },
      },
    );
  return (
    <>
      <SectionHeader level={3} title={t("legalEntities.sectionLogo")} />
      {src && (
        <CompanyLogo
          name={entity.display_name}
          src={src}
          fallback={<Avatar identity={entity.id} name={entity.display_name} />}
        />
      )}
      <FileDropzone
        label={t(src ? "legalEntities.logoReplace" : "legalEntities.logoAdd")}
        hint={t("legalEntities.logoHint")}
        emptyLabel={t("legalEntities.logoEmpty")}
        accept={ACCEPTED_IMAGES}
        disabled={write.isPending}
        onPick={send}
      />
      {src && (
        <Button
          variant="ghost"
          onClick={() => send(null)}
          pending={write.isPending && write.variables?.file === null}
        >
          {t("legalEntities.logoRemove")}
        </Button>
      )}
      <ErrorLine error={write.isError && problemMessageOf(write.error, t)} />
    </>
  );
}

export function CountryField({
  required,
  value,
  onChange,
}: Readonly<{
  required: boolean;
  value: string;
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
      hint={required ? undefined : t("legalEntities.countryInstallationHint")}
      required={required}
    >
      {(control) => (
        <Select
          {...control}
          value={value}
          placeholder={t("legalEntities.countryPlaceholder")}
          options={
            required
              ? options
              : [
                  { value: "", label: t("legalEntities.countryInstallation") },
                  ...options,
                ]
          }
          onChange={onChange}
        />
      )}
    </Field>
  );
}

type TextProps = Readonly<{
  label: string;
  hint?: string;
  required?: boolean;
  maxLength?: number;
  value: string;
  onChange: (value: string) => void;
}>;

export function TextField({
  label,
  hint,
  required,
  maxLength,
  value,
  onChange,
}: TextProps) {
  return (
    <Field label={label} hint={hint} required={required}>
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

function AreaField({ label, hint, value, onChange }: TextProps) {
  return (
    <Field label={label} hint={hint}>
      {(control) => (
        <Textarea
          {...control}
          rows={3}
          maxLength={4000}
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
