import { Check } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Voiceprint } from "../../../components/voiceprint";
import { usePrefersReducedMotion } from "../../../design-system/motion";
import { formatNumber } from "../../../format/format";
import { useLocale, usePlural, useT } from "../../../i18n";
import type { BuildStage } from "./conversation-machine";
import type { CorpusSummary } from "./voice-scenes";

const BUILD_STAGES: readonly BuildStage[] = [
  "snapshot",
  "extract",
  "evaluate",
  "activate",
];

const stageLabelKeys = {
  snapshot: "ob.conv.build.snapshot",
  extract: "ob.conv.build.extract",
  evaluate: "ob.conv.build.evaluate",
  activate: "ob.conv.build.activate",
} as const;

// How often the ring closes a slice of the gap to the server's ceiling, and
// how much of that gap it closes per tick. Timer-driven, not rAF: a
// background tab suspends rAF, and a build the reader tabbed away from must
// still read as moving when they come back.
const TWEEN_TICK_MS = 60;

const TWEEN_EASE = 0.08;

/**
 * The displayed progress, creeping toward `ceiling` (the highest fraction the
 * server has confirmed) instead of jumping to it. It starts at zero on mount,
 * with no earlier value to inherit, and only closes the gap to the current
 * ceiling. So it never exceeds it, and a long stage keeps the ring moving.
 * `prefers-reduced-motion` skips the crawl and reads the ceiling directly.
 */
function useCrawlingProgress(ceiling: number): number {
  const reduced = usePrefersReducedMotion();
  const [displayed, setDisplayed] = useState(0);
  const target = useRef(ceiling);
  target.current = ceiling;

  useEffect(() => {
    if (reduced) {
      return;
    }
    const timer = setInterval(() => {
      setDisplayed((prev) => {
        const gap = target.current - prev;
        return Math.abs(gap) < 0.001 ? target.current : prev + gap * TWEEN_EASE;
      });
    }, TWEEN_TICK_MS);
    return () => clearInterval(timer);
  }, [reduced]);

  return reduced ? ceiling : displayed;
}

/**
 * The build scene: the voiceprint carries the progress ring, with the stages
 * as a checklist beside it. The ceiling comes from the stage the server
 * reports. The number crawls toward it so the ring moves during a stage, and
 * never passes it. It reaches 100 only as the scene leaves the screen.
 */
export function VoiceBuildScene({
  stage,
  summary,
  sources,
}: Readonly<{
  stage: BuildStage | null;
  summary: CorpusSummary | null;
  sources: number;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const reached = stage === null ? -1 : BUILD_STAGES.indexOf(stage);
  // A queued build (no stage yet) shows nothing claimed; each reached stage
  // is one quarter, and the last stage completes only when the build does.
  const ceiling = Math.max(0, reached + 1) / (BUILD_STAGES.length + 1);
  const progress = useCrawlingProgress(ceiling);
  return (
    <div className="ob-scene ob-voice-building">
      {/* The voiceprint is the subject and takes its own column; what the
          build is reading and how far it has got reads down the column beside
          it. Which model is answering is not repeated here: the band's runtime
          chip above names it for every screen in the room. */}
      <div className="ob-voice-building-row">
        {/* Decorative: the stage checklist beside it already carries the
            build's progress in words, so the crawling digits stay out of the
            a11y tree instead of being announced on every tick. */}
        <Voiceprint size="large" busy fraction={progress}>
          <span className="ob-voice-build-pct">
            {formatNumber(Math.round(progress * 100), locale)}
            <small>%</small>
          </span>
        </Voiceprint>
        <div className="ob-voice-building-account">
          <p className="ob-voice-building-meta">
            {plural("ob.conv.voice.buildingMeta", sources, {
              words: formatNumber(summary?.total_words ?? 0, locale),
              sources: formatNumber(sources, locale),
            })}
          </p>
          <ol
            className="ob-conv-stages"
            aria-label={t("ob.conv.voice.stageTitle")}
          >
            {BUILD_STAGES.map((name, index) => (
              <li
                key={name}
                data-state={
                  index < reached
                    ? "done"
                    : index === reached
                      ? "current"
                      : "todo"
                }
              >
                {index < reached && <Check aria-hidden />}
                <span>{t(stageLabelKeys[name])}</span>
              </li>
            ))}
          </ol>
        </div>
      </div>
    </div>
  );
}
