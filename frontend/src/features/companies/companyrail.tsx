// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AddressCountryMismatchBadge } from "../../components/addresscountrymismatch";
import { RecordListsActions } from "../../components/addtoshortlist";
import { ADDRESS_FIELDS, mapCompanyUpdate } from "../../components/companyform";
import {
  formatSizeBand,
  LIFECYCLE_LABELS,
  LIFECYCLE_OPTIONS,
  SIZE_BAND_OPTIONS,
} from "../../components/companylookups";
import { EditAccessRequest } from "../../components/editaccessrequest";
import { type RailEdit, RailField } from "../../components/railfield";
import { useSectionEdit } from "../../components/sectionedit";
import { Badge } from "../../design-system/atoms";
import { InlineChoice } from "../../design-system/inlinechoice";
import { InlineText } from "../../design-system/inlinetext";
import {
  type DetailRow,
  type DetailSection,
  RecordDetails,
} from "../../design-system/recorddetails";
import type { RecordRename } from "../../design-system/recordview";
import { postalLines } from "../../format/address";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCanWriteRecord } from "../../lib/capability";
import { useIndustryOptions } from "../../lib/industryoptions";
import { labelFor } from "../../lib/labelfor";
import { useListsAvailable, useRecordLists } from "../../lib/lists.queries";
import { useRecordCarriesTags } from "../../lib/tags.queries";
import { CompanyContactCard } from "./companycontactcard";
import {
  companyWebsite,
  displayHost,
  useCompanyReadOnlyReason,
} from "./companyheader";
import { useCompanyEdit, useCompanyFieldSave } from "./companyrailedit";
import {
  useOwnershipSection,
  useRegistrationSection,
} from "./companyrailgroups";
import { CompanyTagsSection } from "./companyrailtags";

// The company's details under its head: About, Ownership, Registration and
// Lists as flat groups. Every empty field folds behind one link, and the
// page's read-only sentence sits at the foot.

type Company = components["schemas"]["Company"];
type Company360 = components["schemas"]["Company360"];

type RailProps = Readonly<{
  company: Company;
  view?: Company360;
  // The composite read is still in flight, so the Way in row can tell "still
  // loading" from "nobody has a way in".
  loading: boolean;
  // Why this reader cannot change the account, said once as the footnote. The
  // verbs in the head point at it by `id`.
  refusal?: Readonly<{ id: string; text: string }>;
}>;

export function CompanyRail(props: RailProps) {
  // The lists read answers 404 while lists are switched off, and the server
  // refuses an archived account's lists, so it is asked for only otherwise.
  return useListsAvailable() && !props.company.archived_at ? (
    <ListedRail {...props} />
  ) : (
    <RailBody {...props} />
  );
}

function ListedRail(props: RailProps) {
  const lists = useRecordLists("company", props.company.id);
  const found = lists.data?.data ?? [];
  return (
    <RailBody
      {...props}
      lists={
        <>
          {found.length > 0 && (
            <ul className="co-aside-chips">
              {found.map((list) => (
                <li key={list.id}>
                  <Badge>{list.name}</Badge>
                </li>
              ))}
            </ul>
          )}
          <RecordListsActions
            entityType="company"
            entityId={props.company.id}
          />
        </>
      }
    />
  );
}

function RailBody({
  company,
  view,
  loading,
  refusal,
  lists,
}: RailProps & Readonly<{ lists?: ReactNode }>) {
  const t = useT();
  const gate = useCompanyEditGate(company);
  const edit = useCompanyEdit(company, gate.canEdit);
  const about = useAboutSection(company, view, loading, edit);
  const address = useAddressSection(company, edit);
  const ownership = useOwnershipSection(company, edit);
  const registration = useRegistrationSection(company, edit, gate);
  const sections: DetailSection[] = [
    about.section,
    address.section,
    ownership.section,
    registration.section,
  ];
  if (lists) {
    sections.push({
      key: "lists",
      label: t("lists.record.title"),
      help: t("lists.record.help"),
      children: lists,
    });
  }
  return (
    <>
      <RecordDetails
        sections={sections}
        footnote={
          refusal && (
            <>
              <span id={refusal.id}>{refusal.text}</span>
              <EditAccessRequest kind="company" record={company} />
            </>
          )
        }
      />
      {about.element}
      {address.element}
      {ownership.element}
      {registration.element}
    </>
  );
}

export type Edit = Readonly<{ canEdit: boolean; reason?: string }>;

/**
 * Whether this reader may change the account's own fields, and if not, why.
 * Every surface that edits them (the rail, the Profile tab, the growth-fit
 * checklist) refuses with this one answer.
 */
export function useCompanyEditGate(company: Company): Edit {
  const t = useT();
  const writable = useCanWriteRecord("company", company);
  const reason = useCompanyReadOnlyReason(company);
  return {
    canEdit: writable && !reason,
    reason: reason ?? (writable ? undefined : t("record.notYoursToChange")),
  };
}

/** The account's name as the record's head renames it, with the rail's gate. */
export function useCompanyRename(company: Company): RecordRename {
  const t = useT();
  const edit = useCompanyEditGate(company);
  const save = useCompanyFieldSave(company);
  return {
    label: t("create.displayName"),
    value: company.display_name,
    canEdit: edit.canEdit,
    readOnlyReason: edit.reason,
    onSave: (next) => save(mapCompanyUpdate({ display_name: next }, {})),
  };
}

function isBlank(value: string | null | undefined): boolean {
  return !value?.trim();
}

// One details row per kind of field. Each opens its edit in a dialog (the
// record's edit form, for a field of several inputs) and folds while empty.
export function useRowBuilders(company: Company, edit: Edit) {
  const t = useT();
  const save = useCompanyFieldSave(company);
  // Every write goes through the form's one mapper, which trims, clears an
  // erased value to null and leaves untouched fields out of the patch.
  const write = (key: string) => (next: string) =>
    save(mapCompanyUpdate({ [key]: next }, {}));
  const text = (
    key: "industry" | "legal_name" | "linkedin_url" | "phone" | "email",
    label: MessageKey,
    shape: Readonly<{ type?: "email"; link?: ReactNode }> = {},
  ): DetailRow => ({
    key,
    term: t(label),
    empty: isBlank(company[key]),
    value: (
      <InlineText
        label={t(label)}
        value={company[key] ?? ""}
        type={shape.type}
        link={shape.link}
        placeholder={t("field.unset")}
        canEdit={edit.canEdit}
        readOnlyReason={edit.reason}
        onSave={write(key)}
      />
    ),
  });
  const choice = (
    key: "size_band" | "lifecycle" | "industry",
    label: MessageKey,
    listed: readonly { value: string; label: string }[],
  ): DetailRow => {
    // Industry and size band may be cleared, which the mapper sends as null;
    // lifecycle has its own "unknown" stage instead.
    const options =
      key === "lifecycle"
        ? listed
        : [{ value: "", label: t("field.unset") }, ...listed];
    return {
      key,
      term: t(label),
      empty: !company[key] || company[key] === "unknown",
      value: (
        <InlineChoice
          label={t(label)}
          hideLabel
          value={company[key] ?? ""}
          options={options}
          render={(value) =>
            options.find((option) => option.value === value)?.label ??
            t("field.unset")
          }
          canEdit={edit.canEdit}
          readOnlyReason={edit.reason}
          onSave={write(key)}
        />
      ),
    };
  };
  // The Profile tab's grouped fields, each opening the record's edit form.
  const companyEdit = useCompanyEdit(company, edit.canEdit);
  const compound = (
    key: "parent_company_id" | "relationship_types",
    label: string,
    reading: ReactNode | undefined,
    shape: Readonly<{ link?: boolean }> = {},
  ): DetailRow => ({
    key,
    term: label,
    empty: !reading,
    value: (
      <RailField
        edit={companyEdit}
        keys={[key]}
        label={label}
        reading={reading ?? (edit.canEdit ? undefined : t("field.unset"))}
        link={shape.link && Boolean(reading)}
      />
    ),
  });
  return { text, choice, compound };
}

const ABOUT_KEYS = ["domains", "industry", "size_band", "lifecycle"];
const ADDRESS_KEYS = ADDRESS_FIELDS.map((field) => field.key);

function useAboutSection(
  company: Company,
  view: Company360 | undefined,
  loading: boolean,
  edit: RailEdit,
) {
  const t = useT();
  const label = t("record.details.about");
  const about = useSectionEdit(edit, { label, keys: ABOUT_KEYS });
  const section: DetailSection = {
    key: "about",
    label,
    rows: useAboutRows(company, view, loading),
    edit: about.edit,
  };
  return { section, element: about.element };
}

function useAddressSection(company: Company, edit: RailEdit) {
  const t = useT();
  const label = t("co.address.summary");
  const address = postalLines(company.address);
  const change = useSectionEdit(edit, { label, keys: ADDRESS_KEYS });
  const section: DetailSection = {
    key: "address",
    label,
    edit: change.edit,
    rows: [
      {
        key: "address",
        term: label,
        empty: !address,
        value: (
          <>
            {address}
            {company.address_country_mismatch && (
              <>
                {" "}
                <AddressCountryMismatchBadge
                  mismatch={company.address_country_mismatch}
                />
              </>
            )}
          </>
        ),
      },
    ],
  };
  return { section, element: change.element };
}

// The rows the design reads under About: each set one shows, each empty one
// folds behind "Show N empty fields". The account's other fields live on the
// Profile tab (companyprofiledetails.tsx).
function useAboutRows(
  company: Company,
  view: Company360 | undefined,
  loading: boolean,
): DetailRow[] {
  const t = useT();
  const industries = useIndustryOptions(company.industry);
  const website = companyWebsite(company);
  const wayIn = useWayIn(company, view, loading);
  const tagged = useRecordCarriesTags("company", company.id, view?.tags);
  const choice = (
    key: "size_band" | "lifecycle" | "industry",
    label: MessageKey,
    options: readonly { value: string; label: string }[],
  ): DetailRow => ({
    key,
    term: t(label),
    empty: !company[key] || company[key] === "unknown",
    value:
      options.find((option) => option.value === company[key])?.label ??
      t("field.unset"),
  });
  return [
    // The head already links the site; here the domain is a reading.
    {
      key: "domains",
      term: t("field.domain"),
      empty: !website,
      value: website && displayHost(website),
    },
    choice("industry", "create.industry", industries),
    choice(
      "size_band",
      "co.aside.size",
      SIZE_BAND_OPTIONS.map((band) => ({
        value: band,
        label: formatSizeBand(t, band),
      })),
    ),
    choice(
      "lifecycle",
      "company.lifecycle",
      LIFECYCLE_OPTIONS.map((value) => ({
        value,
        label: t(labelFor(LIFECYCLE_LABELS, value)),
      })),
    ),
    ...(wayIn
      ? [
          {
            key: "wayin",
            term: t("co.aside.wayIn"),
            block: true,
            value: wayIn,
          },
        ]
      : []),
    // An archived company takes no tags, so it has no Tags row to fold.
    ...(company.archived_at
      ? []
      : [
          {
            key: "tags",
            term: t("tags.panelTitle"),
            block: true,
            empty: !tagged,
            value: (
              <CompanyTagsSection
                company={company}
                companyId={company.id}
                bare
              />
            ),
          },
        ]),
  ];
}

// The best way in: the contact the server names as the strongest route,
// drawn as the card the Overview's Contacts card draws.
function useWayIn(
  company: Company,
  view: Company360 | undefined,
  loading: boolean,
): ReactNode | undefined {
  const id = loading ? undefined : view?.strength?.contributor_contact_id;
  const contact = id
    ? view?.contacts?.data.find((row) => row.contact_id === id)
    : undefined;
  if (!contact || company.archived_at) {
    return undefined;
  }
  return (
    <CompanyContactCard contact={contact} companyName={company.display_name} />
  );
}
