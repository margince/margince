// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The review queue for links an extension suggests: which record a message
// belongs to. Margince's own "which deal" question waits here too, for the one
// colleague it asks, and names no unit. Nothing is filed until a reader
// accepts it. The panel draws nothing while the queue is empty.

import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelRow } from "../../design-system/panel";
import { Row, Stack } from "../../design-system/stack";
import { formatMoneyWhole } from "../../format/format";
import { formatDayMonthNearYear } from "../../format/format.dates";
import { type Translator, useLocale, useT } from "../../i18n";
import { extensionTitle } from "../../lib/extensions";
import { problemStatus } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import {
  type LinkSuggestion,
  useDecideLinkSuggestion,
  useLinkSuggestions,
} from "./linksuggestions.queries";
import { AFTER_THE_DAY } from "./worklist.layout";

export function LinkSuggestionsPanel() {
  const t = useT();
  const suggestions = useLinkSuggestions();
  if (suggestions.isError) {
    const refused = problemStatus(suggestions.error) === 403;
    return (
      <p className="t-caption">
        {t(
          refused ? "linkSuggestions.noAccess" : "linkSuggestions.unavailable",
        )}
      </p>
    );
  }
  if (!suggestions.data || suggestions.data.length === 0) {
    return null;
  }
  return (
    <Panel className={AFTER_THE_DAY} title={t("linkSuggestions.title")}>
      {suggestions.data.map((suggestion) => (
        <LinkSuggestionRow key={suggestion.id} suggestion={suggestion} />
      ))}
    </Panel>
  );
}

/**
 * A suggested deal's value and close date, which help pick between deals. A
 * value the reader's role withholds arrives empty and is left out.
 */
function dealFacts(
  suggestion: LinkSuggestion,
  t: Translator,
  locale: Parameters<typeof formatMoneyWhole>[2],
  zone: string,
): string {
  const facts: string[] = [];
  if (suggestion.deal_amount_minor != null && suggestion.deal_currency) {
    facts.push(
      formatMoneyWhole(
        suggestion.deal_amount_minor,
        suggestion.deal_currency,
        locale,
      ),
    );
  }
  if (suggestion.deal_expected_close_date) {
    facts.push(
      t("deal.closes", {
        date: formatDayMonthNearYear(
          suggestion.deal_expected_close_date,
          locale,
          zone,
          new Date(),
        ),
      }),
    );
  }
  return facts.join(" · ");
}

function LinkSuggestionRow({
  suggestion,
}: Readonly<{ suggestion: LinkSuggestion }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const facts = dealFacts(suggestion, t, locale, zone);
  const decide = useDecideLinkSuggestion();
  const deciding = (verb: "accept" | "reject") =>
    decide.isPending && decide.variables?.verb === verb;
  return (
    <PanelRow>
      <Stack gap="2">
        <p className="t-body">
          {t("linkSuggestions.target", {
            subject:
              suggestion.activity_subject ?? t("linkSuggestions.noSubject"),
            record: suggestion.entity_name ?? "",
            kind: t(`linkSuggestions.kind.${suggestion.entity_type}`),
          })}
        </p>
        {facts && <p className="t-caption">{facts}</p>}
        <p className="t-caption">
          {t("linkSuggestions.reason", {
            unit: suggestion.unit
              ? extensionTitle(suggestion.unit, locale)
              : t("linkSuggestions.byMargince"),
            reason: suggestion.reason,
          })}
        </p>
        <ErrorLine error={decide.error} />
        <Row gap="2">
          <Button
            variant="primary"
            pending={deciding("accept")}
            disabled={decide.isPending}
            onClick={() => decide.mutate({ id: suggestion.id, verb: "accept" })}
          >
            {t("linkSuggestions.accept")}
          </Button>
          <Button
            variant="ghost"
            pending={deciding("reject")}
            disabled={decide.isPending}
            onClick={() => decide.mutate({ id: suggestion.id, verb: "reject" })}
          >
            {t("linkSuggestions.reject")}
          </Button>
        </Row>
      </Stack>
    </PanelRow>
  );
}
