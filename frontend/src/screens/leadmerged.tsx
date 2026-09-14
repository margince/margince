import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { EntityRef } from "./entityref";

/**
 * MergedLeadPanel is what a merged-away lead's page is FOR.
 *
 * The merge archives the loser and points it at the survivor, and it leaves
 * the ladder status exactly where it stood — a lead merged away while it was
 * being worked is still `contacted`. So without this the page draws an open
 * lead nobody is working, or, once it reads as terminal at all, reads as
 * *disqualified*: a human judged this prospect not worth pursuing. Merged says
 * something else entirely — it was the same prospect as another one — and a
 * rep who reads the wrong one draws the wrong conclusion about their own
 * pipeline.
 *
 * The pointer is the useful half. A reader who learns the lead was merged
 * wants the lead it was merged INTO, which is where its timeline, its consent
 * and its score now live. Same shape as the promoted lead's link to its
 * contact, for the same reason.
 */
export function MergedLeadPanel({
  mergedIntoId,
}: Readonly<{ mergedIntoId: string }>) {
  const t = useT();
  return (
    <Panel title={t("lead.mergedTitle")}>
      <PanelBody>
        <div className="lead-stack">
          <p className="t-body">{t("lead.mergedBody")}</p>
          <p className="t-body">
            <EntityRef kind="lead" id={mergedIntoId} />
          </p>
        </div>
      </PanelBody>
    </Panel>
  );
}
