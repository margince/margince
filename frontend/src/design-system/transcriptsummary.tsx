import type { components } from "../api/schema";
import { usePlural, useT } from "../i18n";
import { Badge } from "./atoms";
import { Eyebrow } from "./eyebrow";
import { useLatestTranscriptRead } from "./transcriptread";
import { formatSourceLines } from "./trust";

type TranscriptSummaryPoint = components["schemas"]["TranscriptSummaryPoint"];

/**
 * TranscriptSummary is the short summary a reading wrote onto a meeting, drawn
 * above its transcript with the AI mark. Each point names the lines it was read
 * from. It draws nothing until a reading has written one, and a reading whose
 * summary was refused leaves the earlier one in place.
 */
export function TranscriptSummary({
  activityId,
}: Readonly<{ activityId: string }>) {
  const t = useT();
  const latest = useLatestTranscriptRead(activityId);
  // A failed re-read hides the summary rather than keep words an erasure may
  // have removed.
  const summary = latest.isError ? undefined : latest.data?.summary;
  if (!summary) {
    return null;
  }
  return (
    <section
      className="transcript-summary"
      aria-label={t("transcriptread.summaryTitle")}
    >
      <p className="transcript-status">
        <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
        <span className="t-caption">{t("transcriptread.summaryTitle")}</span>
      </p>
      <SummaryPoints
        title={t("transcriptread.keyPoints")}
        points={summary.key_points}
      />
      {summary.next_steps.length > 0 && (
        <SummaryPoints
          title={t("transcriptread.nextSteps")}
          points={summary.next_steps}
        />
      )}
    </section>
  );
}

function SummaryPoints({
  title,
  points,
}: Readonly<{ title: string; points: TranscriptSummaryPoint[] }>) {
  const plural = usePlural();
  return (
    <>
      <Eyebrow as="h3">{title}</Eyebrow>
      <ul className="transcript-summary-points">
        {points.map((point) => (
          <li key={`${point.source_lines.join(",")}:${point.text}`}>
            {point.text}{" "}
            <span className="t-caption">
              {plural("trust.evidenceLine", point.source_lines.length, {
                lines: formatSourceLines(point.source_lines),
              })}
            </span>
          </li>
        ))}
      </ul>
    </>
  );
}
