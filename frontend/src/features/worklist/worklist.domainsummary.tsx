// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../../design-system/applink";
import { Heading } from "../../design-system/heading";
import { usePlural, useT } from "../../i18n";
import { recordRoute } from "../../lib/entity";
import { routeHash } from "../../lib/router";
import {
  type DomainQuestionView,
  domainQuestionHeading,
  domainQuestionReasonKey,
  SUGGESTION_BASIS,
} from "./worklist.domaincopy";

// What a company question says before its verbs: the question, who wrote, why
// Margince is asking and the company the evidence points to. The row and the
// review drawer both draw it, so the two say one thing.
export function DomainQuestionSummary({
  view,
}: Readonly<{ view: DomainQuestionView }>) {
  const t = useT();
  const plural = usePlural();
  const suggestion = view.context?.suggested_company;
  return (
    <>
      <Heading size="small" as="h3">
        {domainQuestionHeading(view, t, plural)}
      </Heading>
      <DomainQuestionOrigin view={view} />
      <p>{t(domainQuestionReasonKey(view), { domain: view.domain })}</p>
      {suggestion ? (
        <p>
          {t("worklist.domainQuestion.suggested")}{" "}
          <AppLink href={recordHref("company", suggestion.id)}>
            {suggestion.name}
          </AppLink>
          <br />
          <span className="t-caption">
            {t(SUGGESTION_BASIS[suggestion.basis])}
          </span>
        </p>
      ) : (
        <p className="t-caption">{t("worklist.domainQuestion.noSuggestion")}</p>
      )}
    </>
  );
}

// Where the question came from: the contacts the reader may open, or a plain
// statement that the sender is not one of them.
function DomainQuestionOrigin({
  view,
}: Readonly<{ view: DomainQuestionView }>) {
  const t = useT();
  const contacts = view.context?.contacts ?? [];
  if (contacts.length === 0) {
    return (
      <p className="t-caption">
        {t(
          view.context?.hidden_senders
            ? "worklist.domainQuestion.originHidden"
            : "worklist.domainQuestion.origin",
          { domain: view.domain },
        )}
      </p>
    );
  }
  return (
    <p className="t-caption">
      {t("worklist.domainQuestion.originContacts", { domain: view.domain })}{" "}
      {contacts.map((contact, index) => (
        <span key={contact.id}>
          {index > 0 && ", "}
          <AppLink href={recordHref("contact", contact.id)}>
            {contact.name}
          </AppLink>
        </span>
      ))}
    </p>
  );
}

function recordHref(kind: "company" | "contact", id: string) {
  const route = recordRoute(kind, id);
  return route && routeHash(route);
}
