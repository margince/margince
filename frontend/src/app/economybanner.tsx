import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { throwProblem } from "../screens/common";
import { settingsHref } from "../screens/settingsrouting";
import { useCan, useCanWrite } from "./capability";
import { routeHash } from "./router";

export function EconomyBanner() {
  const t = useT();
  const canRead = useCan("ai_budget", "read");
  const enabled = useCanWrite("ai_budget", "update") && canRead;
  const previousBand = useRef<string | undefined>(undefined);
  const [occurrence, setOccurrence] = useState(0);
  const [dismissedOccurrence, setDismissedOccurrence] = useState<string | null>(
    null,
  );
  const query = useQuery({
    queryKey: ["ai-budget"],
    enabled,
    staleTime: 5 * 60_000,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/budget");
      if (error) throwProblem(error);
      if (!data) throw new Error("AI allowance unavailable");
      return data;
    },
  });
  const band = query.data?.band;
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
        actions={
          <a href={routeHash(settingsHref("usage"))}>{t("aibanner.link")}</a>
        }
        dismiss={{
          label: t("aibanner.dismiss"),
          onDismiss: () => setDismissedOccurrence(occurrenceKey),
        }}
      />
    </div>
  );
}
