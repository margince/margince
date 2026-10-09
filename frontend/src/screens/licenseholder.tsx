import { CalendarClock } from "lucide-react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatDateAbbrev } from "../format/format";
import { useLocale, useT } from "../i18n";

// Who holds the license, and how long it lasts. The card above the seat meter:
// two subjects, two cards — who this license belongs to, then what it grants.
//
// Every claim except the identifiers and the expiry is optional. A license
// issued before those claims existed verifies exactly like any other, so each
// row renders only when the token carries it. Empty rows would tell a reader
// that something is missing from THEIR license rather than from the vocabulary
// it was issued under.
//
// Two states are stated here, and they are ordered against the seat warning
// below by what they cost. Expiry stops the installation eventually; being over
// the seat count never stops anything. So the expiry notice lives here, above.
// Neither interrupts: both are true as the page renders, and a settings tab
// that announced them on every visit would teach a reader to ignore all three.

type LicenseHolder = components["schemas"]["LicenseHolder"];

export function LicenseHolderCard({
  holder,
}: Readonly<{ holder: LicenseHolder }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // The record zone is the one zone this product renders dates in, so an
  // expiry and an activity beside it can never be read in two different zones.
  const expiry = formatDateAbbrev(holder.expiry, locale, recordZone);

  return (
    <Panel title={t("license.holder.title")}>
      {(holder.in_grace || holder.renewal_due) && (
        <PanelBody>
          {holder.in_grace ? (
            // The license stopped being current and still works. This is the one
            // state upstream calls out: it passes today and will stop passing.
            <Callout
              kind="standing"
              tone="danger"
              title={t("license.grace.title")}
            >
              {t("license.grace.body", { expiry })}
            </Callout>
          ) : (
            holder.renewal_due && (
              // Amber, because nothing has gone wrong yet; the calendar glyph,
              // because this notice is about a date rather than a severity.
              <Callout
                kind="standing"
                tone="warning"
                icon={CalendarClock}
                title={t("license.renewal.title")}
              >
                {t("license.renewal.body", { expiry })}
              </Callout>
            )
          )}
        </PanelBody>
      )}
      {/* No control: a license changes only with the deployment. Older
          licenses lack the optional claims, so an absent claim gets no row. */}
      <SettingList bleed="settings">
        {holder.company && (
          <SettingRow
            label={t("license.holder.company")}
            value={holder.company}
            control={null}
          />
        )}
        {(holder.contact_name || holder.contact_email) && (
          <SettingRow
            label={t("license.holder.contact")}
            value={
              <>
                {holder.contact_name}
                {holder.contact_name && holder.contact_email && " · "}
                {holder.contact_email}
              </>
            }
            control={null}
          />
        )}
        <SettingRow
          label={t("license.holder.installation")}
          value={holder.subject}
          control={null}
        />
        <SettingRow
          // The label follows the fact. "Valid until" beside a date that has
          // already passed states the opposite of what the notice above says.
          label={
            holder.in_grace
              ? t("license.holder.expiredOn")
              : t("license.holder.validUntil")
          }
          value={expiry}
          control={null}
        />
        <SettingRow
          label={t("license.holder.id")}
          // The support reference, verbatim: somebody reads it aloud or
          // copies it into a ticket.
          value={<span>{holder.id}</span>}
          control={null}
        />
      </SettingList>
    </Panel>
  );
}
