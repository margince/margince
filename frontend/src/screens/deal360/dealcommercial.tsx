import type { components } from "../../api/schema";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import {
  type AcquisitionSource,
  acquisitionLabel,
} from "../acquisitionsources.queries";
import { MOTION_OPTIONS, PRIORITY_OPTIONS } from "./dealcommercialfields";

type Deal = components["schemas"]["Deal"];

/**
 * The deal's commercial context in the record's side pane: why it exists, how
 * much a person says it matters, and which channel brought it.
 *
 * Renders nothing when all three are unset. An empty panel would say the deal
 * has no commercial context when what is true is that nobody has recorded one,
 * and the difference matters on a page whose whole job is to show what is
 * known.
 */
export function DealCommercial({
  deal,
  sources,
}: Readonly<{ deal: Deal; sources?: AcquisitionSource[] }>) {
  const t = useT();
  const motion = MOTION_OPTIONS.find(
    (o) => o.value && o.value === deal.commercial_motion,
  );
  const priority = PRIORITY_OPTIONS.find(
    (o) => o.value && o.value === deal.priority,
  );
  const source = acquisitionLabel(deal.acquisition_source, sources);
  if (!motion && !priority && !source) {
    return null;
  }
  return (
    <Panel title={t("deal.commercialContext")}>
      <PanelBody>
        {/* The same labelled-pair list the custom-field card and the company
            rail use, so a reader meets one shape for "field: value" across
            every record page. */}
        <dl className="firmo">
          {motion && (
            <div>
              <dt className="t-eyebrow">{t("deal.motion")}</dt>
              <dd>{t(motion.label)}</dd>
            </div>
          )}
          {/* The WORD, never a colour alone: a reader who cannot tell the
              tones apart still reads the value. */}
          {priority && (
            <div>
              <dt className="t-eyebrow">{t("deal.priority")}</dt>
              <dd>{t(priority.label)}</dd>
            </div>
          )}
          {source && (
            <div>
              <dt className="t-eyebrow">{t("deal.acquisitionSource")}</dt>
              <dd>{source}</dd>
            </div>
          )}
        </dl>
      </PanelBody>
    </Panel>
  );
}
