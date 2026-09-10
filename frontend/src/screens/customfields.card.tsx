// The read-only custom fields on a record 360. Mirrors the firmographics card
// (organizations.tsx): a labeled section + a `dl.firmo` list, evidence-or-omit
// — a field with no stored value is absent, and a record with no custom values
// renders nothing at all rather than an empty card.

import { OffsiteLink } from "../design-system/offsitelink";
import { Panel, PanelBody } from "../design-system/panel";
import { useLocale, useT } from "../i18n";
import {
  customFieldDisplay,
  customFieldHref,
  useObjectCustomFields,
} from "./customfields.form";
import type { CfObject } from "./customfields.logic";

export function CustomFieldsPanel({
  object,
  record,
}: Readonly<{ object: CfObject; record: Record<string, unknown> }>) {
  const t = useT();
  const { locale } = useLocale();
  const cf = useObjectCustomFields(object);
  const boolLabels = { yes: t("field.yes"), no: t("field.no") };

  const rows = cf.fields
    .map((field) => ({
      field,
      value: customFieldDisplay(field, record[field.column_name], {
        locale,
        boolLabels,
      }),
    }))
    .filter((row): row is { field: typeof row.field; value: string } =>
      Boolean(row.value),
    );

  if (rows.length === 0) {
    return null;
  }

  return (
    <Panel title={t("cf.formSection")}>
      <PanelBody>
        <dl className="firmo">
          {rows.map(({ field, value }) => {
            // A value that IS a web address becomes a link, and everything else
            // stays text. A field holding the ticket, the wiki page or the ERP
            // entry a record belongs to is the commonest thing anybody puts in a
            // text field, and copy-and-paste was the only way to follow it. The
            // scheme check inside customFieldHref is what keeps this from turning
            // a stored string into something executable on click, and it is
            // asked HERE rather than left to the link: a refused value in this
            // column is one of many plain cells, so it must not wear the link
            // affordance the primitive keeps on the text it declines to follow.
            const href = customFieldHref(value);
            return (
              <div key={field.column_name}>
                <dt className="t-eyebrow">{field.label}</dt>
                <dd>
                  {href ? (
                    <OffsiteLink href={href}>{value}</OffsiteLink>
                  ) : (
                    value
                  )}
                </dd>
              </div>
            );
          })}
        </dl>
      </PanelBody>
    </Panel>
  );
}
