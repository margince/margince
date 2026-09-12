// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The company form: what a create or an edit sends, and the fields that
// collect it.
//
// Its own module rather than the screen's, because none of it is the screen's.
// `companyheader.tsx` and the create dialog both build the same body from the
// same field list, and while this lived inside a 2,900-line screen file the
// only way to reach it was to import from that screen — which is how a
// form-mapping layer ended up as a screen's public surface.
//
// Nothing here renders. The field DEFINITIONS are data the renderer walks, so
// this is `.ts` and stays that way: a JSX import here would be the screen
// creeping back in.

import { api } from "../api/client";
import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import {
  LIFECYCLE_LABELS,
  LIFECYCLE_OPTIONS,
  RELATIONSHIP_TYPE_LABELS,
  SIZE_BAND_OPTIONS,
} from "./companylookups";
import type { CreateField, FormRows } from "./create";
import { splitMultiselectValue } from "./create";

type Company = components["schemas"]["Company"];
type CreateCompanyRequest = components["schemas"]["CreateCompanyRequest"];
type UpdateCompanyRequest = components["schemas"]["UpdateCompanyRequest"];

function stringField(value: unknown): string {
  return typeof value === "string" ? value : "";
}

// Merge-target search (P-2): mirrors searchContactsTargets (contacts.tsx) — the
// caller filters out the source row.
export async function searchCompanyTargets(
  q: string,
): Promise<{ id: string; name: string }[]> {
  const { data, error } = await api.GET("/companies", {
    params: { query: { q, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((candidate) => ({
    id: candidate.id,
    name: candidate.display_name,
  }));
}

function asSizeBand(
  value: string | undefined,
): CreateCompanyRequest["size_band"] {
  return (SIZE_BAND_OPTIONS as readonly string[]).includes(value ?? "")
    ? (value as CreateCompanyRequest["size_band"])
    : undefined;
}

// The repeatable `domains` rows → the wire `domains[]` shape, shared by the
// create body and the edit patch: blank rows drop out, the domain lowercases,
// and the row's primary radio (a string "true"/"") becomes the boolean flag.
// An empty result is `undefined` — on create that means "no domains", on
// update the field is omitted so the stored set stays untouched (never
// silently cleared).
function mapDomainRows(rows: FormRows): CreateCompanyRequest["domains"] {
  const domains = mapDomainRowsReplaceSet(rows);
  return domains.length > 0 ? domains : undefined;
}

type DomainPatch = NonNullable<UpdateCompanyRequest["domains"]>;

// The edit-patch form of the repeatable domains field: always the concrete
// desired set (possibly empty), so a caller can send [] to clear every domain.
// Blank rows drop; the primary radio ("true"/"") becomes the boolean flag.
function mapDomainRowsReplaceSet(rows: FormRows): DomainPatch {
  return (rows.domains ?? [])
    .filter((row) => (row.domain ?? "").trim().length > 0)
    .map((row) => ({
      domain: row.domain.trim().toLowerCase(),
      is_primary: row.is_primary === "true",
    }));
}

// Order-independent set equality: an edit that leaves the domains untouched
// omits the field (sparse PATCH), while any real change — including clearing
// to empty — sends the replace-set.
function sameDomainSet(a: DomainPatch, b: DomainPatch): boolean {
  if (a.length !== b.length) {
    return false;
  }
  const key = (d: DomainPatch[number]) => `${d.domain}:${d.is_primary ? 1 : 0}`;
  const seen = new Set(a.map(key));
  return b.every((d) => seen.has(key(d)));
}

// Builds the create-company request body: `domains[]` rows carry
// `{domain, is_primary}` keyed off the repeatable rows channel, scalar
// fields trim to undefined when blank.
export function mapCompanyBody(
  values: Record<string, string>,
  rows: FormRows,
): CreateCompanyRequest {
  return {
    display_name: values.display_name.trim(),
    legal_name: values.legal_name?.trim() || undefined,
    industry: values.industry?.trim() || undefined,
    size_band: asSizeBand(values.size_band),
    domains: mapDomainRows(rows),
    source: "manual",
  };
}

// Builds the PATCH body: the scalar UpdateCompanyRequest fields plus the
// domains replace-set from the edit modal's repeatable rows. Domains are sent
// only when the set actually changed from `currentDomains` — an untouched edit
// omits the field (sparse PATCH), and clearing every row sends [] (clear all),
// the two cases the contract's "absent = untouched" vs "[] = clear" distinguish.
export function mapCompanyUpdate(
  values: Record<string, unknown>,
  rows: FormRows,
  currentDomains: Company["domains"] = [],
): UpdateCompanyRequest {
  const desired = mapDomainRowsReplaceSet(rows);
  const current: DomainPatch = (currentDomains ?? []).map((domain) => ({
    domain: domain.domain,
    is_primary: domain.is_primary,
  }));
  const body: UpdateCompanyRequest = {
    display_name: stringField(values.display_name).trim() || undefined,
    legal_name: stringField(values.legal_name).trim() || undefined,
    industry: stringField(values.industry).trim() || undefined,
    size_band: asSizeBand(stringField(values.size_band)),
    owner_id: stringField(values.owner_id).trim() || undefined,
  };
  if (!sameDomainSet(desired, current)) {
    body.domains = desired;
  }
  const lifecycle = stringField(values.lifecycle).trim();
  if (lifecycle) {
    body.lifecycle = lifecycle as NonNullable<
      UpdateCompanyRequest["lifecycle"]
    >;
  }
  // Always sent when the field was rendered, even empty: this is a replace-set,
  // and "the user cleared every type" is an edit, not an absence. The form
  // channel joins a multiselect into one comma string, so an empty string is
  // the honest empty set.
  if (values.relationship_types !== undefined) {
    body.relationship_types = splitMultiselectValue(
      stringField(values.relationship_types),
    ) as NonNullable<UpdateCompanyRequest["relationship_types"]>;
  }
  // Nullable rather than trim-to-undefined, and for the same reason the
  // relationship set is: clearing a LinkedIn URL is an edit. `|| undefined`
  // would read a deletion as "the caller did not mention it" and put the old
  // value straight back.
  if (values.linkedin_url !== undefined) {
    body.linkedin_url = stringField(values.linkedin_url).trim() || null;
  }
  const address = addressPatch(values);
  if (address) {
    body.address = address;
  }
  return body;
}

// The six columns behind Address, flattened into form fields. The wire shape is
// one nested object; the form channel is flat string values, so the two are
// mapped at the boundary (addressFrom / addressPatch) rather than teaching the
// form about nesting for one record type.
const ADDRESS_FIELDS: CreateField[] = [
  { key: "address_line1", label: "create.addressLine1" },
  { key: "address_line2", label: "create.addressLine2" },
  { key: "address_postal_code", label: "create.postalCode" },
  { key: "address_city", label: "create.city" },
  { key: "address_region", label: "create.region" },
  { key: "address_country", label: "create.country" },
];

// addressFrom prefills the six flat fields from the record's nested address.
export function addressFrom(
  address: Company["address"],
): Record<string, string> {
  return {
    address_line1: address?.line1 ?? "",
    address_line2: address?.line2 ?? "",
    address_postal_code: address?.postal_code ?? "",
    address_city: address?.city ?? "",
    address_region: address?.region ?? "",
    address_country: address?.country ?? "",
  };
}

// addressPatch folds the six flat fields back into the wire's nested object.
//
// A cleared field is sent as null rather than omitted: the caller had the value
// on screen and erased it, which is an edit. Omitting it would silently keep
// what the record held — the failure mode where a user deletes a line, saves,
// and finds it back on reload.
//
// The whole object is omitted only when the form never rendered the fields at
// all, so a surface that does not offer the address cannot blank one.
function addressPatch(
  values: Record<string, unknown>,
): UpdateCompanyRequest["address"] | undefined {
  if (values.address_line1 === undefined) {
    return undefined;
  }
  const field = (key: string) => stringField(values[key]).trim() || null;
  return {
    line1: field("address_line1"),
    line2: field("address_line2"),
    postal_code: field("address_postal_code"),
    city: field("address_city"),
    region: field("address_region"),
    // ISO-3166 alpha-2, and the server compares on the canonical spelling, so
    // "de" typed in lower case is the same country as "DE".
    country: stringField(values.address_country).trim().toUpperCase() || null,
  };
}

export const companyCreateFields: CreateField[] = [
  { key: "display_name", label: "create.displayName", required: true },
  { key: "legal_name", label: "create.legalName" },
  { key: "industry", label: "create.industry" },
  {
    key: "size_band",
    label: "create.sizeBand",
    type: "select",
    options: SIZE_BAND_OPTIONS.map((band) => ({ value: band, label: band })),
  },
  {
    key: "domains",
    label: "company.domains",
    type: "repeatable",
    addLabel: "field.addDomain",
    rowFields: [{ key: "domain", label: "field.domain", required: true }],
    primaryKey: "is_primary",
  },
];

// The edit form, built per-render because the owner options are the live user
// roster.
//
// Stage and relationship types ARE here now: the retired classification could
// not be edited from anywhere, because the update contract carried no such
// field.
// Where the account stands with us: lives in companylookups.ts, same reason
// as LIFECYCLE_LABELS and SIZE_BAND_OPTIONS above — the rail's Details grid
// builds a lifecycle picker off the same wire order, and a second copy here

// What it is to us has no rail counterpart today, so it stays local.
export const RELATIONSHIP_TYPE_OPTIONS = [
  "customer",
  "partner",
  "supplier",
  "investor",
  "portfolio_company",
  "competitor",
  "other",
] as const;

// t is threaded in because the option LABELS are catalog keys, not words: the
// field renderer prints option.label as given, so an untranslated key reaches
// the reader as "company.lifecycle.customer".
export function companyEditFields(
  owners: readonly { id: string; display_name: string }[],
  hasOwner: boolean,
  t: (key: MessageKey) => string,
): CreateField[] {
  return [
    { key: "display_name", label: "create.displayName", required: true },
    { key: "legal_name", label: "create.legalName" },
    { key: "industry", label: "create.industry" },
    {
      key: "size_band",
      label: "create.sizeBand",
      type: "select",
      options: SIZE_BAND_OPTIONS.map((band) => ({ value: band, label: band })),
    },
    // Who is accountable for this account. It defaults to whoever created the
    // record and stays there until someone changes it — which, until now,
    // nothing on this page let them do.
    //
    // Required exactly when the account HAS an owner: an optional select
    // offers a blank option, and `UpdateCompanyRequest.owner_id` cannot
    // carry "unassign" — a null is indistinguishable from an omitted field on
    // the wire. Offering the blank would take the answer and drop it. An
    // account with no owner yet keeps the blank, because there it is the
    // truthful current state rather than an edit we cannot make.
    {
      key: "owner_id",
      label: "co.pulse.owner",
      type: "select",
      required: hasOwner,
      options: owners.map((user) => ({
        value: user.id,
        label: user.display_name,
      })),
    },
    // Where the account stands, and what it is to us — the two questions the
    // retired classification tried to answer with one value, and the reason
    // neither was editable from this page at all.
    {
      key: "lifecycle",
      label: "company.lifecycle",
      type: "select",
      options: LIFECYCLE_OPTIONS.map((value) => ({
        value,
        label: t(LIFECYCLE_LABELS[value]),
      })),
    },
    {
      key: "relationship_types",
      label: "company.relationshipTypes",
      type: "multiselect",
      options: RELATIONSHIP_TYPE_OPTIONS.map((value) => ({
        value,
        label: t(RELATIONSHIP_TYPE_LABELS[value]),
      })),
    },
    // The company's own LinkedIn page. A canonical column since ADR-0085,
    // not a custom field, because it carries identity semantics — matching,
    // dedupe, enrichment — and the contact side already treats it that way. The
    // server normalizes what is pasted, so a URL copied from any tab of the
    // company page resolves to the one spelling.
    { key: "linkedin_url", label: "create.linkedinUrl" },
    // Where the company actually is. It has been in the API since the record
    // existed and reachable from no form on this page, so a rep who knew the
    // address had nowhere to put it.
    ...ADDRESS_FIELDS,
    {
      key: "domains",
      label: "company.domains",
      type: "repeatable",
      addLabel: "field.addDomain",
      rowFields: [{ key: "domain", label: "field.domain", required: true }],
      primaryKey: "is_primary",
    },
  ];
}

export async function createCompany(
  values: Record<string, string>,
  rows: FormRows | undefined,
  customFields: Record<string, unknown>,
  t: (key: MessageKey) => string,
): Promise<Company> {
  const { data, error } = await api.POST("/companies", {
    body: { ...mapCompanyBody(values, rows ?? {}), ...customFields },
  });
  if (error) {
    throwProblem(error, t);
  }
  return data;
}
