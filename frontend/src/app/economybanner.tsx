import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { Callout } from "../design-system/callout";
import { calendarDay } from "../format/calendarday";
import { viewerZone } from "../format/timezone";
import { useT } from "../i18n";
import { throwProblem } from "../screens/common";
import { useCan } from "./capability";

export function EconomyBanner() {
  const t = useT();
  // GET /ai/usage asks for ai_diagnostics:read (ai/usage.go). It used to ride
  // automation:update — an AI reading behind another object's WRITE grant — and
  // three client gates were left on that object when the server moved. The
  // object is named here rather than a role so a rebinding fails in a test
  // instead of 403-ing in a browser.
  const enabled = useCan("ai_diagnostics", "read");
  const previousBand = useRef<string | undefined>(undefined);
  const [occurrence, setOccurrence] = useState(0);
  const [dismissedOccurrence, setDismissedOccurrence] = useState<string | null>(
    null,
  );
  const query = useQuery({
    queryKey: ["ai-usage-band"],
    enabled,
    staleTime: 5 * 60_000,
    queryFn: async () => {
      // A one-day window, and the day is the reader's own.
      //
      // The window is here to keep the response small — this banner reads only
      // `budget.band`, and an unbounded query returns every day of the month
      // with its per-task rows. It does NOT decide the band: that is a
      // month-to-date figure the server computes for itself (ai.Meter's
      // MonthTokens), and `from`/`to` never reach it. So the reader's day
      // rather than UTC's is honesty about what the word "today" means here,
      // not a fix for a wrong band — the band was never wrong.
      const today = calendarDay(new Date(), viewerZone());
      const { data, error } = await api.GET("/ai/usage", {
        params: { query: { from: today, to: today } },
      });
      if (error) throwProblem(error);
      if (!data?.budget) throw new Error("malformed AI usage response");
      return data;
    },
  });
  const band = query.data?.budget?.band;
  useEffect(() => {
    if (band !== previousBand.current) {
      previousBand.current = band;
      setOccurrence((value) => value + 1);
    }
  }, [band]);
  const occurrenceKey = band ? `${band}:${occurrence}` : null;
  // The banner is advisory; errors stay on the accountable Settings card.
  if (
    !enabled ||
    query.isError ||
    !band ||
    band === "normal" ||
    dismissedOccurrence === occurrenceKey
  ) {
    return null;
  }
  return (
    // The band decides the tone: a queued workspace is being refused work
    // right now, where a degraded one is still serving and only warning.
    // `.appbanner` is the SHELL's hook, not the notice's: the content column
    // reserves the top bar's height only when no banner is mounted above it
    // (`.main:not(:has(> .appbanner))`, app/shell.css), so the class stays on a
    // wrapper the screen owns rather than on a primitive with no layout seam.
    <div className="appbanner">
      <Callout
        tone={band === "queued" ? "danger" : "warn"}
        kind="standing"
        title={
          band === "queued"
            ? t("aibanner.queued")
            : band === "degraded"
              ? t("aibanner.degraded")
              : t("aibanner.unknown")
        }
        actions={<a href="#/settings/ai">{t("aibanner.link")}</a>}
        dismiss={{
          label: t("aibanner.dismiss"),
          onDismiss: () => setDismissedOccurrence(occurrenceKey),
        }}
      />
    </div>
  );
}
