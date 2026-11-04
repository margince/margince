// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Card } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { keepFor } from "./retention.logic";

type PrivacyInformationPage = components["schemas"]["PrivacyInformationPage"];
type Controller = components["schemas"]["PrivacyInformationController"];
type Retention = components["schemas"]["PrivacyInformationRetention"];

// The cards of the privacy page past the source and the purposes. Each one
// names codes the server chose, so nothing here decides what is disclosed.

export const BASIS_LABEL: Readonly<
  Record<PrivacyInformationPage["purpose_bases"][number], MessageKey>
> = {
  consent: "privacynotice.basis.consent",
  legitimate_interests: "privacynotice.basis.legitimateInterests",
  contract: "privacynotice.basis.contract",
};

const CATEGORY_LABEL: Readonly<
  Record<PrivacyInformationPage["data_categories"][number], MessageKey>
> = {
  name: "privacynotice.category.name",
  job_title: "privacynotice.category.jobTitle",
  employer: "privacynotice.category.employer",
  email: "privacynotice.category.email",
  phone: "privacynotice.category.phone",
  correspondence: "privacynotice.category.correspondence",
};

const RECIPIENT_LABEL: Readonly<
  Record<PrivacyInformationPage["recipients"][number], MessageKey>
> = {
  colleagues: "privacynotice.recipient.colleagues",
  service_providers: "privacynotice.recipient.serviceProviders",
};

// Only the scopes the server discloses to a reader; any other scope it could
// send is named by the general sentence below the list instead.
const SCOPE_LABEL: Partial<Record<Retention["scope"], MessageKey>> = {
  activity: "privacynotice.retention.scope.activity",
  "activity/transcript": "privacynotice.retention.scope.activityTranscript",
  raw_capture: "privacynotice.retention.scope.rawCapture",
  "ai_call_payload/content": "privacynotice.retention.scope.aiCallPayload",
  "contact/no_consent_no_deal":
    "privacynotice.retention.scope.contactNoConsentNoDeal",
  "lead/unconverted": "privacynotice.retention.scope.leadUnconverted",
};

const ACTION_LABEL: Readonly<Record<Retention["action"], MessageKey>> = {
  archive: "privacynotice.retention.archive",
  anonymize: "privacynotice.retention.anonymize",
  erase: "privacynotice.retention.erase",
};

/** Who answers for the data, as the installation stated it. */
export function ControllerCard({
  controller,
}: Readonly<{ controller?: Controller }>) {
  const t = useT();
  if (!controller) {
    return null;
  }
  return (
    <Card>
      <Heading size="large" className="t-h3">
        {t("privacynotice.controller.title")}
      </Heading>
      {controller.legal_name ? (
        <p className="t-body">{controller.legal_name}</p>
      ) : null}
      {controller.postal_address ? (
        <p className="t-body confirm-lines">{controller.postal_address}</p>
      ) : null}
      {controller.privacy_contact ? (
        <p className="t-body">
          {t("privacynotice.controller.contact", {
            contact: controller.privacy_contact,
          })}
        </p>
      ) : null}
    </Card>
  );
}

/** The kinds of data held, and who sees them. */
export function HeldCard({ card }: Readonly<{ card: PrivacyInformationPage }>) {
  const t = useT();
  return (
    <>
      {card.data_categories.length > 0 ? (
        <Card>
          <Heading size="large" className="t-h3">
            {t("privacynotice.categories.title")}
          </Heading>
          <ul className="confirm-fields">
            {card.data_categories.map((category) => (
              <li key={category} className="t-body">
                {t(CATEGORY_LABEL[category])}
              </li>
            ))}
          </ul>
        </Card>
      ) : null}
      <Card>
        <Heading size="large" className="t-h3">
          {t("privacynotice.recipients.title")}
        </Heading>
        <ul className="confirm-fields">
          {card.recipients.map((recipient) => (
            <li key={recipient} className="t-body">
              {t(RECIPIENT_LABEL[recipient])}
            </li>
          ))}
        </ul>
      </Card>
    </>
  );
}

/** The retention rules that act on the reader's data, then the rest. */
export function RetentionCard({
  retention,
}: Readonly<{ retention: readonly Retention[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  // The window reads as the retention settings screen shows it.
  const windowOf = (days: number) => {
    const window = keepFor(days);
    return plural(window.unit, window.count, {
      count: formatNumber(window.count, locale),
    });
  };
  const named = retention.flatMap((rule) => {
    const scope = SCOPE_LABEL[rule.scope];
    return scope ? [{ rule, scope }] : [];
  });
  return (
    <Card>
      <Heading size="large" className="t-h3">
        {t("privacynotice.retention.title")}
      </Heading>
      {named.length > 0 ? (
        <ul className="confirm-fields">
          {named.map(({ rule, scope }) => (
            <li key={rule.scope} className="t-body">
              {t(ACTION_LABEL[rule.action], {
                scope: t(scope),
                days: windowOf(rule.retain_days),
              })}
            </li>
          ))}
        </ul>
      ) : null}
      <p className="t-body">{t("privacynotice.retention.rest")}</p>
    </Card>
  );
}

/** The right to object, stated apart from the other rights. */
export function ObjectionCard({ route }: Readonly<{ route?: string }>) {
  const t = useT();
  return (
    <Card>
      <Heading size="large" className="t-h3">
        {t("privacynotice.objection.title")}
      </Heading>
      <p className="t-body">{t("privacynotice.objection.body")}</p>
      <p className="t-body">
        {route
          ? t("privacynotice.objection.how", { route })
          : t("privacynotice.rights.how")}
      </p>
    </Card>
  );
}
