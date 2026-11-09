import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import {
  Button,
  Checkbox,
  EmptyState,
  SearchField,
} from "../../design-system/atoms";
import { useSettledValue } from "../../design-system/debouncedsearch";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { kindLabel, SENDERS_KEY } from "./capture-senderquery";
import { SenderDecisions } from "./capture-senders";
export function SenderCorrections() {
  const t = useT();
  const [typed, setTyped] = useState("");
  const [decision, setDecision] = useState("");
  const [overrides, setOverrides] = useState(false);
  const [browse, setBrowse] = useState(false);
  const q = useSettledValue(typed.trim());
  const active = Boolean(q || decision || overrides || browse);
  const read = useInfiniteQuery({
    queryKey: [...SENDERS_KEY, { q, decision, overrides }],
    enabled: active,
    initialPageParam: "",
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/capture/senders", {
          params: {
            query: {
              q,
              limit: 100,
              decision: decision || undefined,
              overrides,
              cursor: pageParam || undefined,
            },
          },
        }),
      ),
    getNextPageParam: (page) => page.next_cursor || undefined,
  });
  const rows = [
    ...new Map(
      (read.data?.pages ?? [])
        .flatMap((page) => page.data)
        .map((row) => [row.address, row]),
    ).values(),
  ];
  return (
    <div className="form-stack">
      <p>{t("senders.sub")}</p>
      <SearchField
        aria-label={t("senders.search")}
        value={typed}
        onChange={(event) => setTyped(event.target.value)}
      />
      <Select
        aria-label={t("senders.decisionFilter")}
        value={decision}
        onChange={setDecision}
        options={[
          { value: "", label: t("senders.allDecisions") },
          { value: "business", label: t("senders.kind.business") },
          { value: "keep_out", label: t("senders.keepOut") },
          ...Object.entries(kindLabel).map(([value, key]) => ({
            value,
            label: t(key),
          })),
        ]}
      />
      <Checkbox
        label={t("senders.myOverrides")}
        checked={overrides}
        onChange={(event) => setOverrides(event.target.checked)}
      />
      {!active ? (
        <Button onClick={() => setBrowse(true)}>{t("senders.browse")}</Button>
      ) : read.isPending ? (
        <p role="status">{t("common.loading")}</p>
      ) : rows.length === 0 ? (
        <EmptyState>{t("heldThreads.noMatch")}</EmptyState>
      ) : (
        <SenderDecisions rows={rows} />
      )}
      <ErrorLine error={read.error} />
      {read.hasNextPage && (
        <Button
          pending={read.isFetchingNextPage}
          onClick={() => void read.fetchNextPage()}
        >
          {t("heldThreads.loadMore")}
        </Button>
      )}
    </div>
  );
}
