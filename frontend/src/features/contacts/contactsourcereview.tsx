// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";

export function ContactSourceReview({
  view,
  onAction,
}: Readonly<{
  view: components["schemas"]["Contact360"];
  onAction: (action: components["schemas"]["ContactMomentAction"]) => void;
}>) {
  const t = useT();
  const first = view.privacy_source_reviews?.[0];
  if (!first) return null;
  return (
    <Panel title={t("notice.source.title")}>
      <PanelBody>
        <p>{t("notice.source.contactExplanation")}</p>
        {(view.privacy_source_reviews?.length ?? 0) > 1 && (
          <p>{t("notice.source.more")}</p>
        )}
        <Button
          onClick={() =>
            onAction({
              kind: "open_record",
              state: "available",
              label: t("notice.source.review"),
              destination: { surface: "privacy_notice", entity_id: first },
            })
          }
        >
          {t("notice.source.review")}
        </Button>
      </PanelBody>
    </Panel>
  );
}
