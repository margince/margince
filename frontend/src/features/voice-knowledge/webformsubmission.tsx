// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Check, X } from "lucide-react";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { ContactLink } from "../../design-system/contactlink";
import { type Fact, FactList } from "../../design-system/factlist";
import { OffsiteLink } from "../../design-system/offsitelink";
import { Panel, PanelBody } from "../../design-system/panel";
import { webUrl } from "../../format/weburl";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { WriteToRecord } from "../../lib/writeto";

type Submission = components["schemas"]["WebFormSubmission"];
type ActivityLink = components["schemas"]["ActivityLink"];

/**
 * What a website form sent, field by field. The page is a link and the consent
 * a badge with its policy version. Every value is the visitor's text, so none
 * of it is read as markdown.
 */
export function WebFormSubmission({
  sent,
  filed,
}: Readonly<{ sent: Submission; filed: readonly ActivityLink[] }>) {
  const t = useT();
  const record = writeToRecord(filed);
  const text = (key: string, term: string, value: string | undefined) =>
    value ? [{ key, term, value }] : [];
  const facts: Fact[] = [
    ...(sent.message
      ? [
          {
            key: "message",
            term: t("activity.webForm.message"),
            value: (
              <span style={{ whiteSpace: "pre-wrap" }}>{sent.message}</span>
            ),
            block: true,
          },
        ]
      : []),
    ...text("name", t("activity.webForm.name"), sent.full_name),
    {
      key: "email",
      term: t("activity.webForm.email"),
      value: record ? (
        <ContactLink kind="email" value={sent.email} record={record} />
      ) : (
        sent.email
      ),
    },
    ...text("phone", t("activity.webForm.phone"), sent.phone),
    ...text("company", t("activity.webForm.company"), sent.company),
    ...text("title", t("activity.webForm.jobTitle"), sent.title),
    ...sent.extra.map((field, index) => ({
      key: `extra-${index}`,
      term: field.name,
      value: field.value,
    })),
    ...linkFact("page", t("activity.webForm.page"), sent.page_url),
    ...linkFact("referrer", t("activity.webForm.referrer"), sent.referrer),
    ...sent.campaign.map((tag, index) => ({
      key: `campaign-${index}`,
      term: campaignTerm(tag.name, t),
      value: tag.value,
    })),
  ];
  if (sent.consent) {
    facts.push({
      key: "consent",
      term: t("activity.webForm.consent"),
      value: <ConsentBadges consent={sent.consent} />,
      note: `“${sent.consent.wording}”`,
    });
  }
  return (
    <>
      <Panel title={t("activity.webForm.title")}>
        <PanelBody>
          <FactList facts={facts} flat />
        </PanelBody>
      </Panel>
    </>
  );
}

// The five standard `utm_` names read as words; any other keeps its own name.
const CAMPAIGN_TERMS = new Map<string, MessageKey>([
  ["utm_source", "activity.webForm.utmSource"],
  ["utm_medium", "activity.webForm.utmMedium"],
  ["utm_campaign", "activity.webForm.utmCampaign"],
  ["utm_term", "activity.webForm.utmTerm"],
  ["utm_content", "activity.webForm.utmContent"],
]);

function campaignTerm(name: string, t: ReturnType<typeof useT>): string {
  const key = CAMPAIGN_TERMS.get(name);
  return key ? t(key) : name;
}

function ConsentBadges({
  consent,
}: Readonly<{ consent: NonNullable<Submission["consent"]> }>) {
  const t = useT();
  return (
    <>
      {consent.ticked ? (
        <Badge tone="success" icon={Check}>
          {t("activity.webForm.consentGiven")}
        </Badge>
      ) : (
        <Badge tone="warning" icon={X}>
          {t("activity.webForm.consentMissing")}
        </Badge>
      )}{" "}
      {consent.policy_version && (
        <Badge>
          {t("activity.webForm.policy", { version: consent.policy_version })}
        </Badge>
      )}
    </>
  );
}

// A page shows as its host and path, the way a reader names one. The link
// keeps the whole address; a value that is no web address stays text.
function linkFact(key: string, term: string, address: string | undefined) {
  if (!address) {
    return [];
  }
  return [
    {
      key,
      term,
      value: <OffsiteLink href={address}>{shortAddress(address)}</OffsiteLink>,
    },
  ];
}

function shortAddress(address: string): string {
  const url = webUrl(address);
  if (!url) {
    return address;
  }
  return `${url.host}${url.pathname === "/" ? "" : url.pathname}`;
}

// The record a reply to the visitor is filed under: the lead or contact the
// note belongs to.
function writeToRecord(
  filed: readonly ActivityLink[],
): WriteToRecord | undefined {
  const target = filed.find(
    (link) => link.entity_type === "lead" || link.entity_type === "contact",
  );
  return target
    ? { entityType: target.entity_type, entityId: target.entity_id }
    : undefined;
}
