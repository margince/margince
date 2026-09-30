import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import {
  EvidenceMark,
  type EvidenceMarkSource,
} from "../design-system/evidencemark";
import { providerBrandName } from "../design-system/provider-mark";
import { formatDateAbbrev } from "../format/format";
import { useLocale, useT } from "../i18n";

// What a data provider was PAID to put on a contact, marked where it stands on
// the record. The server decides which values still hold what the purchase
// wrote (`bought_fields`), so a value a colleague has since corrected carries
// no mark and a value "Delete bought data" took back is gone with its mark.

type Contact = components["schemas"]["Contact"];
export type BoughtField = components["schemas"]["BoughtField"];

/** The mark every bought value carries: sold by a named third party, on a
 *  date. `connector` rather than `agent` — nothing inferred this, somebody
 *  sold it to us. */
export function useBoughtSource(): (
  provider: string,
  at: string | null | undefined,
) => EvidenceMarkSource {
  const { locale } = useLocale();
  const zone = useRecordZone();
  return (provider, at) => ({
    provenance: {
      kind: "connector",
      connector: providerBrandName(provider) ?? provider,
    },
    at: at ? formatDateAbbrev(at, locale, zone) : null,
  });
}

/** The contact's bought values by target (`title`, `phone:<id>`, ...). */
export function boughtFields(
  contact: Contact,
): ReadonlyMap<string, BoughtField> {
  return new Map((contact.bought_fields ?? []).map((f) => [f.target, f]));
}

/**
 * The mark for a value that is itself a control — a link, an inline edit, a
 * record button. EvidenceMark is a button too, so it sits BESIDE the value and
 * shows the word "bought"; `subject` names the value for a screen reader.
 * Nothing renders when the value was not bought.
 */
export function BoughtMark({
  bought,
  subject,
}: Readonly<{ bought: BoughtField | undefined; subject: string }>) {
  const t = useT();
  const source = useBoughtSource();
  if (!bought) {
    return null;
  }
  return (
    <EvidenceMark
      value={t("evidence.bought")}
      subject={subject}
      source={source(bought.provider, bought.applied_at)}
    />
  );
}

/** A bought value that is plain text, underlined in place. */
export function BoughtText({
  value,
  bought,
}: Readonly<{
  value: string;
  bought: Pick<BoughtField, "provider" | "applied_at"> | undefined;
}>) {
  const source = useBoughtSource();
  return (
    <EvidenceMark
      value={value}
      source={bought ? source(bought.provider, bought.applied_at) : undefined}
    />
  );
}
