import { Fragment } from "react";
import {
  Avatar,
  Badge,
  Checkbox,
  Disclosure,
} from "../../../design-system/atoms";
import { Heading } from "../../../design-system/heading";
import { type Evidence, EvidenceChip } from "../../../design-system/trust";
import { formatNumber } from "../../../format/format";
import { type Locale, useLocale, useT } from "../../../i18n";
import { coldFieldLabel } from "../../settings/coldfieldlabels";
import type { CompanyFieldName } from "../onboarding";
import { normalizeUrl } from "../onboarding";
import {
  CapNotice,
  type FactSelection,
  saveDisabled,
} from "../onboarding-facts";
import type { ReviewRow } from "./company-review-state";
import type {
  CompanyConfirmCardProps,
  CompanySiteRead,
  SiteContact,
  SiteFact,
} from "./confirm-card";
import {
  CONTACTS_KEY,
  FACTS_KEY,
  FieldRow,
  type FrozenGroup,
  groupDomId,
  jumpToRow,
} from "./confirm-card.row";

// The one field with no review row: no group carries "website", the gate's
// field. Its domain renders as plain text rather than a jump link.
function companyWebsiteDomain(website: string): string {
  const trimmed = website.trim();
  return trimmed === "" ? "" : normalizeUrl(trimmed).host || trimmed;
}

type CompanyCardFact = {
  label: string;
  value: string;
  field: CompanyFieldName | null;
};

// The identity summary that leads the review: brand mark, name and the
// fields that identify the business. It is not another editable form: each
// value with a row below is a jump link to it, the rest are plain text.
export function CompanyIdentityCard({
  draft,
  companyId,
}: Pick<CompanyConfirmCardProps, "draft" | "companyId">) {
  const t = useT();
  const legalName = draft.values.legal_name.trim();
  const name = draft.values.display_name.trim() || legalName;
  if (name === "") {
    return null;
  }
  const facts: CompanyCardFact[] = [];
  if (legalName !== "" && legalName !== name) {
    facts.push({
      label: coldFieldLabel("legal_name", t),
      value: legalName,
      field: "legal_name",
    });
  }
  const domain = companyWebsiteDomain(draft.values.website);
  if (domain !== "") {
    facts.push({
      label: t("ob.conv.triage.companyWebsite"),
      value: domain,
      field: null,
    });
  }
  const headquarters = draft.values.registered_address.trim();
  if (headquarters !== "") {
    facts.push({
      label: coldFieldLabel("registered_address", t),
      value: headquarters,
      field: "registered_address",
    });
  }
  const industry = draft.values.industry.trim();
  if (industry !== "") {
    facts.push({
      label: coldFieldLabel("industry", t),
      value: industry,
      field: "industry",
    });
  }
  return (
    <div className="ob-company-card">
      {/* The contract carries no logo/favicon field for a company; the
          monogram is the floor, not a fallback for a missing fetch. Keyed as
          the digest's is: the company once it exists, else the name. */}
      <Avatar name={name} identity={companyId ?? name} size="md" />
      <div className="ob-company-card-body">
        <Heading size="medium" className="t-h3">
          {name}
        </Heading>
        {facts.length > 0 && (
          <dl className="ob-company-card-facts">
            {facts.map((fact) => {
              const { field } = fact;
              return (
                <div key={fact.label}>
                  <dt className="t-eyebrow">{fact.label}</dt>
                  <dd>
                    {field === null ? (
                      fact.value
                    ) : (
                      <button type="button" onClick={() => jumpToRow(field)}>
                        {fact.value}
                      </button>
                    )}
                  </dd>
                </div>
              );
            })}
          </dl>
        )}
      </div>
    </div>
  );
}

// A contact is evidence-or-omit like any other finding. The contract requires
// a snippet and a source for each, so this only guards against an empty
// quote. Whitespace counts as empty: a chip of three spaces claims false proof.
function contactEvidence(contact: SiteContact): Evidence | null {
  return contact.evidence_snippet.trim() === "" ||
    contact.evidence_url.trim() === ""
    ? null
    : { snippet: contact.evidence_snippet, source: contact.evidence_url };
}

// One contact the read found: name, role and any published address or
// network profile. The source page is one toggle away, in the collapsed
// evidence chip a profile field uses.
function ContactRow({ contact }: Readonly<{ contact: SiteContact }>) {
  const evidence = contactEvidence(contact);
  return (
    <li className="ob-triage-contact">
      <span className="ob-triage-contact-name">{contact.name}</span>
      <span className="t-sub">{contact.role}</span>
      {contact.published_email && (
        <span className="t-caption">{contact.published_email}</span>
      )}
      {contact.linkedin_url && (
        <span className="t-caption">{contact.linkedin_url}</span>
      )}
      {evidence && <EvidenceChip evidence={evidence} collapsed />}
    </li>
  );
}

/**
 * The contacts the read found, in a section of their own. They are company
 * facts (who to talk to), like an office or a service line. Nothing here is
 * for a human to resolve, so the section has no outstanding count. It states
 * what was found, or that nothing was.
 */
function ContactsGroupSection({
  contacts,
  t,
}: Readonly<{ contacts: readonly SiteContact[]; t: ReturnType<typeof useT> }>) {
  const { locale } = useLocale();
  return (
    <section id={groupDomId(CONTACTS_KEY)} className="ob-triage-group">
      <div className="ob-triage-group-head">
        <Heading size="medium">{t("ob.conv.triage.contactsLabel")}</Heading>
        {contacts.length > 0 && (
          <span className="t-caption">
            {t("ob.conv.triage.contactsCount", {
              count: formatNumber(contacts.length, locale),
            })}
          </span>
        )}
      </div>
      {contacts.length === 0 ? (
        <p className="ob-triage-contacts-empty t-sub">
          {t("ob.conv.triage.contactsEmpty")}
        </p>
      ) : (
        <ul className="ob-triage-contacts-rows">
          {contacts.map((contact) => (
            <ContactRow
              key={`${contact.name}:${contact.evidence_url}`}
              contact={contact}
            />
          ))}
        </ul>
      )}
    </section>
  );
}

// The wire's field enum in its declared order, so the same read always groups
// its facts the same way. Only types a read produced get a heading; the set
// fixes the order, not what shows.
const FACT_FIELD_ORDER: readonly SiteFact["field"][] = [
  "founded_year",
  "employee_range",
  "phone",
  "contact_email",
  "location",
  "service",
  "product",
  "capability",
  "served_industry",
  "company_size",
  "geography",
  "language",
  "certification",
  "partner",
  "named_customer",
  "technology",
  "quantified_outcome",
];

function factsByType(facts: readonly SiteFact[]): readonly Readonly<{
  field: SiteFact["field"];
  facts: readonly SiteFact[];
}>[] {
  return FACT_FIELD_ORDER.map((field) => ({
    field,
    facts: facts.filter((fact) => fact.field === field),
  })).filter((group) => group.facts.length > 0);
}

// One finding on one line: value, confidence, source one toggle away, and the
// save/drop control the classic form's fact grid uses.
function FactRow({
  fact,
  selection,
}: Readonly<{
  fact: SiteFact;
  selection: FactSelection;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const selected = selection.isSelected(fact);
  const evidence: Evidence = {
    snippet: fact.evidence_snippet,
    source: fact.evidence_url,
  };
  return (
    <li className="ob-triage-fact">
      {/* Which findings are kept is a set, so this is the product's one tick. Its
          label is hidden because the value stands beside it in the row. */}
      <Checkbox
        className="ob-triage-fact-toggle"
        checked={selected}
        aria-label={t("ob.facts.rowSave", { fact: fact.value })}
        disabled={saveDisabled(selection, selected)}
        onChange={() => selection.toggle(fact)}
        label=""
      />
      <span className="ob-triage-fact-value">{fact.value}</span>
      <span className="ob-triage-fact-meta">
        <span className="ob-triage-score t-caption">
          {formatNumber(Math.round(fact.confidence * 100), locale)}
        </span>
        <EvidenceChip evidence={evidence} collapsed />
      </span>
    </li>
  );
}

// Above this many rows a type is folded shut. A site read can return a hundred
// facts, mostly one type (services, named customers), which would bury the
// small groups that carry a decision. Folding four rows saves nothing.
const FACT_GROUP_OPEN_MAX = 6;

// One type's facts under their own heading, so a hundred rows stay skimmable.
// The reader sees the shape of what was found (three services, one
// certification) first. The count always shows; a long type's rows fold.
function FactTypeGroup({
  field,
  facts,
  selection,
  t,
}: Readonly<{
  field: SiteFact["field"];
  facts: readonly SiteFact[];
  selection: FactSelection;
  t: ReturnType<typeof useT>;
}>) {
  const { locale } = useLocale();
  return (
    <Disclosure
      className="ob-triage-fact-type"
      open={facts.length <= FACT_GROUP_OPEN_MAX}
      summary={
        <>
          {coldFieldLabel(field, t)}
          <span className="ob-triage-fact-type-count">
            <Badge>{formatNumber(facts.length, locale)}</Badge>
          </span>
        </>
      }
    >
      <ul className="ob-triage-fact-rows">
        {facts.map((fact) => (
          <FactRow key={fact.value_key} fact={fact} selection={selection} />
        ))}
      </ul>
    </Disclosure>
  );
}

/**
 * Every fact the read produced, grouped by type, as a section rather than an
 * appendix. Each type's name and count shows from the start. Ticks write
 * through the `FactSelection` the classic form uses, so accepting the review
 * sends the keys the reader chose, including those in unopened groups.
 */
function FactsGroupSection({
  facts,
  selection,
  locale,
  t,
}: Readonly<{
  facts: readonly SiteFact[];
  selection: FactSelection;
  locale: Locale;
  t: ReturnType<typeof useT>;
}>) {
  return (
    <section id={groupDomId(FACTS_KEY)} className="ob-triage-group">
      <div className="ob-triage-group-head">
        <Heading size="medium">{t("ob.conv.triage.factsLabel")}</Heading>
        <span className="t-caption">
          {t("ob.factsSelected", {
            selected: formatNumber(selection.selectedCount, locale),
            total: formatNumber(facts.length, locale),
          })}
        </span>
      </div>
      <p className="ob-sub">{t("ob.factsSub")}</p>
      {/* The thread beside this pane owns no cap sentence of its own; this
          card draws the ceiling and a second live region on the same
          boundary would read the sentence twice. */}
      <CapNotice atCap={selection.atCap} locale={locale} live={false} />
      {factsByType(facts).map(({ field, facts: typeFacts }) => (
        <FactTypeGroup
          key={field}
          field={field}
          facts={typeFacts}
          selection={selection}
          t={t}
        />
      ))}
    </section>
  );
}

// One field group's board: its head (filled/total) and its rows in the order
// frozen at mount. A divider marks where work rows give way to solid ones.
function FieldGroupSection({
  group,
  rowByField,
  setField,
  t,
}: Readonly<{
  group: FrozenGroup;
  rowByField: ReadonlyMap<CompanyFieldName, ReviewRow>;
  setField: (field: CompanyFieldName, value: string) => void;
  t: ReturnType<typeof useT>;
}>) {
  const { locale } = useLocale();
  const ordered = group.order
    .map((field) => rowByField.get(field))
    .filter((row): row is ReviewRow => row !== undefined);
  const filled = ordered.filter((row) => row.value.trim() !== "").length;
  const solidCount = group.order.length - group.workCount;
  return (
    <section id={groupDomId(group.key)} className="ob-triage-group">
      <div className="ob-triage-group-head">
        <Heading size="medium">{t(group.labelKey)}</Heading>
        <span className="t-caption">
          {formatNumber(filled, locale)}/
          {formatNumber(group.order.length, locale)}
        </span>
      </div>
      <ul className="ob-conv-confirm-fields ob-triage-rows">
        {ordered.map((row, index) => (
          <Fragment key={row.field}>
            {/* The settled cluster sits under its own quiet rule, so the eye
                can stop reading a group the moment it crosses it. */}
            {index === group.workCount &&
              group.workCount > 0 &&
              solidCount > 0 && (
                <li className="ob-triage-solid-divider" aria-hidden>
                  <span className="t-eyebrow">
                    {t("ob.conv.triage.looksSolid", {
                      count: formatNumber(solidCount, locale),
                    })}
                  </span>
                </li>
              )}
            <FieldRow
              row={row}
              setField={setField}
              defaultExpanded={group.openByDefault.has(row.field)}
            />
          </Fragment>
        ))}
      </ul>
    </section>
  );
}

// Which section body a slot in the frozen layout renders: the two
// non-field slots first, the ordinary field group otherwise. One switch,
// so the board's own render stays a plain map over `frozen` rather than a
// nested ternary repeated at the call site.
export function GroupBody({
  group,
  rowByField,
  setField,
  read,
  facts,
  factSelection,
  locale,
  t,
}: Readonly<{
  group: FrozenGroup;
  rowByField: ReadonlyMap<CompanyFieldName, ReviewRow>;
  setField: (field: CompanyFieldName, value: string) => void;
  /** Null on a proposal-only render, where there is no contacts section: "no
   * contacts found" would be a guess rather than a finding. */
  read: CompanySiteRead | null;
  facts: readonly SiteFact[];
  factSelection: FactSelection;
  locale: Locale;
  t: ReturnType<typeof useT>;
}>) {
  if (group.key === CONTACTS_KEY) {
    return read === null ? null : (
      <ContactsGroupSection contacts={read.contacts} t={t} />
    );
  }
  if (group.key === FACTS_KEY) {
    return (
      <FactsGroupSection
        facts={facts}
        selection={factSelection}
        locale={locale}
        t={t}
      />
    );
  }
  return (
    <FieldGroupSection
      group={group}
      rowByField={rowByField}
      setField={setField}
      t={t}
    />
  );
}
