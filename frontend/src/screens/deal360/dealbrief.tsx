import { useState } from "react";
import { useShowDetails } from "../../app/pageaside";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import "./deal360.css";

/** How much of the brief shows before the reader asks for the rest. */
const COLLAPSED_LINES = 6;

/**
 * The deal's human-authored brief, on the record page.
 *
 * Kept visually distinct from the generated status card above it: that one is
 * written by an assembler and can be regenerated, this one is what a colleague
 * typed and nothing overwrites it. A reader who cannot tell them apart cannot
 * tell which parts of the page they may correct.
 *
 * An UNBRIEFED deal shows the panel and says the field is empty, rather than
 * rendering nothing. It used to vanish, on the reasoning that the record's
 * edit form already offers the field — but a reader looking at the page cannot
 * see a form they have not opened, and every deal without a brief therefore
 * looked like a product with no such field at all. A panel that names what it
 * holds is how somebody learns the brief exists.
 */
export function DealBrief({
  brief,
  readOnly = false,
}: Readonly<{
  brief?: string | null;
  // The page's own answer to whether this deal takes writes. The panel does
  // not re-derive it: the server refuses an unauthorized write whatever this
  // says, and a second implementation of the gate here is the defect rather
  // than the protection.
  readOnly?: boolean;
}>) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);
  const showDetails = useShowDetails("description");
  const text = brief?.trim();
  const canWrite = !readOnly && Boolean(showDetails);
  if (!text) {
    return (
      <Panel
        title={t("deal.brief")}
        actions={
          canWrite ? (
            <Button variant="ghost" onClick={showDetails}>
              {t("deal.briefAdd")}
            </Button>
          ) : undefined
        }
      >
        <PanelBody>
          <SurfaceState
            state="empty"
            emptyLabel={t("deal.briefEmpty")}
            emptyDetail={t("deal.briefEmptyDetail")}
            loadingLabel={t("deal.brief")}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      </Panel>
    );
  }
  // The line count decides whether there is anything to expand. Counting lines
  // rather than characters because the clamp is a line clamp: a long single
  // paragraph wraps and is clamped, and asking about its length would offer
  // "Read more" on text already fully shown.
  const clampable = text.split("\n").length > COLLAPSED_LINES;
  const clamped = clampable && !expanded;
  return (
    <Panel
      title={t("deal.brief")}
      actions={
        canWrite ? (
          <Button variant="ghost" onClick={showDetails}>
            {t("deal.briefEdit")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <p
          className={clamped ? "t-body d360-brief-clamped" : "t-body"}
          style={{
            whiteSpace: "pre-wrap",
            ...(clamped ? { WebkitLineClamp: COLLAPSED_LINES } : {}),
          }}
        >
          {text}
        </p>
        {clampable && (
          <Button
            variant="ghost"
            onClick={() => setExpanded((open) => !open)}
            aria-expanded={expanded}
          >
            {expanded ? t("deal.briefLess") : t("deal.briefMore")}
          </Button>
        )}
      </PanelBody>
    </Panel>
  );
}
