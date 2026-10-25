// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AddressCountryMismatchBadge } from "../../components/addresscountrymismatch";
import { mapCompanyUpdate } from "../../components/companyform";
import {
  LIFECYCLE_LABELS,
  LIFECYCLE_OPTIONS,
  SIZE_BAND_OPTIONS,
} from "../../components/companylookups";
import { Badge } from "../../design-system/atoms";
import { InlineChoice } from "../../design-system/inlinechoice";
import { InlineText } from "../../design-system/inlinetext";
import {
  type DetailRow,
  type DetailSection,
  RecordDetails,
} from "../../design-system/recorddetails";
import { INDUSTRY_CODES, industryOptions } from "../../format/industry";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCanWriteRecord } from "../../lib/capability";
import { useListsAvailable, useRecordLists } from "../../lib/lists.queries";
import { CompanyContactCard } from "./companycontactcard";
import {
  companyWebsite,
  displayHost,
  useCompanyReadOnlyReason,
} from "./companyheader";
import {
  type CompoundField,
  CompoundFieldValue,
  useCompanyFieldSave,
} from "./companyrailedit";
import { useOwnershipRows, useRegistrationRows } from "./companyrailgroups";

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
        found.length > 0 && (
          <ul className="co-aside-chips">
            {found.map((list) => (
              <li key={list.id}>
                <Badge>{list.name}</Badge>
              </li>
            ))}
          </ul>
        )
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
  const writable = useCanWriteRecord("company", company);
  const reason = useCompanyReadOnlyReason(company);
  const edit = {
    canEdit: writable && !reason,
    reason: reason ?? (writable ? undefined : t("record.notYoursToChange")),
  };
  const sections: DetailSection[] = [
    {
      key: "about",
      label: t("record.details.about"),
      rows: useAboutRows(company, view, loading, edit),
    },
    {
      key: "ownership",
      label: t("record.details.ownership"),
      rows: useOwnershipRows(company),
    },
    {
      key: "registration",
      label: t("record.registration"),
      rows: useRegistrationRows(company, edit),
    },
  ];
  if (lists) {
    sections.push({
      key: "lists",
      label: t("lists.record.title"),
      children: lists,
    });
  }
  return (
    <RecordDetails
      sections={sections}
      footnote={refusal && <span id={refusal.id}>{refusal.text}</span>}
    />
  );
}

export type Edit = Readonly<{ canEdit: boolean; reason?: string }>;

function isBlank(value: string | null | undefined): boolean {
  return !value?.trim();
}

// One details row per kind of field. Each edits in place (or in a small form,
// for a field of several inputs) and folds while empty.
export function useRowBuilders(company: Company, edit: Edit) {
  const t = useT();
  const save = useCompanyFieldSave(company);
  // Every write goes through the form's one mapper, which trims, clears an
  // erased value to null and leaves untouched fields out of the patch.
  const write = (key: string) => (next: string) =>
    save(mapCompanyUpdate({ [key]: next }, {}));
  const text = (
    key: "industry" | "legal_name" | "linkedin_url",
    label: MessageKey,
  ): DetailRow => ({
    key,
    term: t(label),
    empty: isBlank(company[key]),
    value: (
      <InlineText
        label={t(label)}
        value={company[key] ?? ""}
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
    options: readonly { value: string; label: string }[],
  ): DetailRow => ({
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
  });
  // `flag` sits beside the value rather than inside it, so the edit button's
  // label does not swallow what the flag says.
  const compound = (
    which: CompoundField,
    label: string,
    reading: ReactNode | undefined,
    flag?: ReactNode,
  ): DetailRow => ({
    key: which,
    term: label,
    empty: !reading,
    value: (
      <>
        <CompoundFieldValue
          company={company}
          which={which}
          label={label}
          reading={reading}
          canEdit={edit.canEdit}
          readOnlyReason={edit.reason}
        />
        {flag && <> {flag}</>}
      </>
    ),
  });
  return { text, choice, compound };
}

// The fixed list in the reader's order. A value that is no code, from a row not
// yet repaired, is offered as it is stored so the row never reads as unset.
function useIndustryOptions(company: Company) {
  const t = useT();
  const { locale } = useLocale();
  const options = industryOptions(t, locale);
  const stored = company.industry;
  return stored && !INDUSTRY_CODES.some((code) => code === stored)
    ? [{ value: stored, label: stored }, ...options]
    : options;
}

// The rows the design reads under About: each set one shows, each empty one
// folds behind "Show N empty fields". The account's other fields live on the
// Profile tab (companyprofiledetails.tsx).
function useAboutRows(
  company: Company,
  view: Company360 | undefined,
  loading: boolean,
  edit: Edit,
): DetailRow[] {
  const t = useT();
  const { choice, compound } = useRowBuilders(company, edit);
  const industryOptions = useIndustryOptions(company);
  const website = companyWebsite(company);
  const wayIn = useWayIn(company, view, loading);
  return [
    // The head already links the site; here the value opens the domains form.
    compound("domains", t("field.domain"), website && displayHost(website)),
    choice("industry", "create.industry", industryOptions),
    choice(
      "size_band",
      "co.aside.size",
      SIZE_BAND_OPTIONS.map((band) => ({
        value: band,
        label: band.replace("-", "–"),
      })),
    ),
    choice(
      "lifecycle",
      "company.lifecycle",
      LIFECYCLE_OPTIONS.map((value) => ({
        value,
        label: t(LIFECYCLE_LABELS[value]),
      })),
    ),
    compound(
      "address",
      t("co.address.summary"),
      addressLine(company),
      company.address_country_mismatch && (
        <AddressCountryMismatchBadge
          mismatch={company.address_country_mismatch}
        />
      ),
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
  ];
}

// "Munich, DE": the city and the country, the one line the design reads.
function addressLine(company: Company): string | undefined {
  const address = company.address;
  const parts = [address?.city, address?.country].filter(
    (part): part is string => Boolean(part?.trim()),
  );
  if (parts.length > 0) {
    return parts.join(", ");
  }
  return address?.line1?.trim() || undefined;
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
