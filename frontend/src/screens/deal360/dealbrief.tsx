import { useState } from "react";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";

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
 * Renders NOTHING when the brief is empty rather than an invitation: the edit
 * form already offers the field, and an empty panel on every unbriefed deal is
 * a permanent hole in the page that says nothing.
 */
export function DealBrief({ brief }: Readonly<{ brief?: string | null }>) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);
  const text = brief?.trim();
  if (!text) {
    return null;
  }
  // The line count decides whether there is anything to expand. Counting lines
  // rather than characters because the clamp is a line clamp: a long single
  // paragraph wraps and is clamped, and asking about its length would offer
  // "Read more" on text already fully shown.
  const clampable = text.split("\n").length > COLLAPSED_LINES;
  return (
    <Panel title={t("deal.brief")}>
      <PanelBody>
        <p
          className="t-body"
          style={{
            whiteSpace: "pre-wrap",
            ...(expanded || !clampable
              ? {}
              : {
                  display: "-webkit-box",
                  WebkitLineClamp: COLLAPSED_LINES,
                  WebkitBoxOrient: "vertical",
                  overflow: "hidden",
                }),
          }}
        >
          {text}
        </p>
        {clampable && (
          <Button
            small
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
