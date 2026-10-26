import { useState } from "react";
import { Button } from "../../../design-system/atoms";
import {
  OverviewCard,
  OverviewText,
} from "../../../design-system/overviewcard";
import { useT } from "../../../i18n";
import "./deal360.css";

/** How much of the brief shows before the reader asks for the rest. */
const COLLAPSED_LINES = 6;

/**
 * What this deal is, in the words a colleague typed. Nothing generated
 * overwrites it, which is why it stands apart from How it stands above it.
 * An unbriefed deal keeps the card and says the brief is empty: a card that
 * vanished would read as a product with no such field.
 */
export function DealBrief({
  brief,
  onEdit,
}: Readonly<{
  brief?: string | null;
  // Opens the deal's fields where the brief is written; absent while the
  // deal takes no writes from this reader.
  onEdit?: () => void;
}>) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);
  const text = brief?.trim();
  // A line clamp, so the count is of lines. A long single paragraph wraps
  // and is clamped, and its length alone would offer "Read more" on text
  // already fully shown.
  const clampable =
    Boolean(text) && (text ?? "").split("\n").length > COLLAPSED_LINES;
  const clamped = clampable && !expanded;
  return (
    <OverviewCard
      title={t("deal.brief.title")}
      action={
        onEdit ? (
          <Button variant="link" onClick={onEdit}>
            {text ? t("deal.briefEdit") : t("deal.briefAdd")}
          </Button>
        ) : undefined
      }
      empty={text ? undefined : t("deal.briefEmpty")}
    >
      {text ? (
        <>
          <OverviewText>
            <span
              className={
                clamped ? "d360-brief d360-brief-clamped" : "d360-brief"
              }
              style={clamped ? { WebkitLineClamp: COLLAPSED_LINES } : undefined}
            >
              {text}
            </span>
          </OverviewText>
          {clampable && (
            <Button
              variant="link"
              onClick={() => setExpanded((open) => !open)}
              aria-expanded={expanded}
            >
              {expanded ? t("deal.briefLess") : t("deal.briefMore")}
            </Button>
          )}
        </>
      ) : undefined}
    </OverviewCard>
  );
}
