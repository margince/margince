import { hashKey, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";

type GrowthFit = components["schemas"]["CompanyGrowthFit"];

type Reassessed = Readonly<{
  companyId: string;
  changed: boolean;
  band: GrowthFit["band"];
  present: number;
  expected: number;
}>;

// What "Assess again" did, said once it answers. With the same inputs the
// model gives the same reading, and a card that only moves its timestamp reads
// as a press that did nothing.
export function useReassess(companyId: string) {
  const queryClient = useQueryClient();
  const key = ["company-growth-fit", companyId];
  const [outcome, setOutcome] = useState<Reassessed | null>(null);
  const reassess = useMutation({
    mutationFn: async () => {
      return unwrap(
        await api.POST("/companies/{id}/growth-fit", {
          params: { path: { id: companyId } },
        }),
      );
    },
    onMutate: () => setOutcome(null),
    onSuccess: async (data) => {
      // A background read started before the press would land after this
      // write and put the older reading back.
      await queryClient.cancelQueries({ queryKey: key });
      const before = queryClient.getQueryData<GrowthFit>(key);
      const { present, expected } = data.data_completeness;
      setOutcome({
        companyId,
        changed: !before || readingKey(before) !== readingKey(data),
        band: data.band,
        present,
        expected,
      });
      queryClient.setQueryData(key, data);
    },
  });
  // A panel kept across a move to another company must not carry the first
  // company's outcome.
  return {
    reassess,
    outcome: outcome?.companyId === companyId ? outcome : null,
  };
}

// The time and author of a run change on every press. The reading is the
// rest, so "unchanged" means the panel shows the same thing.
function readingKey(fit: GrowthFit): string {
  const { generated_at, generated_by, ...reading } = fit;
  return hashKey([reading]);
}

export function ReassessOutcome({
  outcome,
  bandLabel,
}: Readonly<{ outcome: Reassessed; bandLabel: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const values = {
    band: bandLabel,
    present: formatNumber(outcome.present, locale),
    expected: formatNumber(outcome.expected, locale),
  };
  return (
    <p className="t-caption">
      {outcome.changed
        ? t("co.growthFit.reassessedChanged", values)
        : t("co.growthFit.reassessedSame", values)}
    </p>
  );
}
