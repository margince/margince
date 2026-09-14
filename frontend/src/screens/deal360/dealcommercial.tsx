import { useState } from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatMoney } from "../../format/format";
import { monthlyEquivalent } from "../../format/recurring";
import { useLocale, useT } from "../../i18n";
import {
  type AcquisitionSource,
  acquisitionLabel,
} from "../acquisitionsources.queries";
import { DealCommercialEdit } from "./dealcommercialedit";
import { MOTION_OPTIONS, PRIORITY_OPTIONS } from "./dealcommercialfields";

type Deal = components["schemas"]["Deal"];

/**
 * The deal's commercial context in the record's side pane: why it exists, how
 * much a colleague says it matters, which channel brought it, and what it is
 * worth per year on a recurring basis.
 *
 * An UNRECORDED deal shows the panel and says the fields are empty, rather
 * than rendering nothing. It used to vanish, on the reasoning that an empty
 * panel would claim the deal has no commercial context — but a reader cannot
 * tell "nobody recorded this" from "this product has no such field" when there
 * is nothing on the page at all, and the second is what they concluded. A
 * panel that names what it holds is how somebody learns these fields exist.
 */
export function DealCommercial({
  deal,
  sources,
  readOnly = false,
}: Readonly<{
  deal: Deal;
  sources?: AcquisitionSource[];
  // The record's own write refusal — archived, or not this caller's to write.
  // Deliberately NOT the advance verb's refusal, which also refuses a closed
  // deal: correcting the motion on a deal lost last week is an ordinary edit
  // the server accepts.
  readOnly?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [editing, setEditing] = useState(false);
  const motion = MOTION_OPTIONS.find(
    (o) => o.value && o.value === deal.commercial_motion,
  );
  const priority = PRIORITY_OPTIONS.find(
    (o) => o.value && o.value === deal.priority,
  );
  const source = acquisitionLabel(deal.acquisition_source, sources);
  // BOTH halves or neither. A figure without its currency cannot be scaled,
  // and the read mask withholds the money reading as one unit, so an ARR
  // arriving without a code is a withheld field rather than a priced deal.
  const arr =
    deal.expected_arr_minor != null && deal.currency
      ? { minor: deal.expected_arr_minor, currency: deal.currency }
      : null;
  const monthly = arr ? monthlyEquivalent(arr.minor) : null;
  const editAction = readOnly ? undefined : (
    <Button variant="ghost" onClick={() => setEditing(true)}>
      {motion || priority || source || arr
        ? t("deal.commercialEdit")
        : t("deal.commercialAdd")}
    </Button>
  );
  const editor = (
    <DealCommercialEdit
      open={editing}
      onClose={() => setEditing(false)}
      deal={deal}
      sources={sources}
    />
  );
  if (!motion && !priority && !source && !arr) {
    return (
      <Panel title={t("deal.commercialContext")} actions={editAction}>
        <PanelBody>
          <SurfaceState
            state="empty"
            emptyLabel={t("deal.commercialEmpty")}
            emptyDetail={t("deal.commercialEmptyDetail")}
            loadingLabel={t("deal.commercialContext")}
          >
            {null}
          </SurfaceState>
        </PanelBody>
        {editor}
      </Panel>
    );
  }
  return (
    <Panel title={t("deal.commercialContext")} actions={editAction}>
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
          {arr && (
            <div>
              <dt className="t-eyebrow">{t("deal.expectedArr")}</dt>
              <dd>{formatMoney(arr.minor, arr.currency, locale)}</dd>
            </div>
          )}
          {/* The monthly reading is DERIVED, and says so when the division
              lost something: twelve of an approximate figure do not add back
              to the year, and a reader who multiplies it should not be
              surprised. */}
          {arr && monthly && (
            <div>
              <dt className="t-eyebrow">{t("deal.monthlyEquivalent")}</dt>
              <dd>
                {monthly.approximate && `${t("deal.monthlyApproximate")} `}
                {formatMoney(monthly.monthlyMinor, arr.currency, locale)}
              </dd>
            </div>
          )}
        </dl>
      </PanelBody>
      {editor}
    </Panel>
  );
}
