// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The review queue for links an extension suggests: which record a message
// belongs to. Nothing is filed until a reader accepts it. The panel draws
// nothing while the queue is empty, so a workspace with no suggesting
// extension never sees it.

import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelRow } from "../../design-system/panel";
import { Row, Stack } from "../../design-system/stack";
import { useT } from "../../i18n";
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
    return <p className="t-caption">{t("linkSuggestions.unavailable")}</p>;
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

function LinkSuggestionRow({
  suggestion,
}: Readonly<{ suggestion: LinkSuggestion }>) {
  const t = useT();
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
        <p className="t-caption">
          {t("linkSuggestions.reason", {
            unit: suggestion.unit,
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
