import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Select } from "../design-system/select";
import { SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { useTagVocabulary } from "./tags.queries";

// The word a connector files what it captures under.
//
// An import already asks this at the start of a run (import.tsx's
// ImportContextTag), and every record the run creates is filed under one
// existing word so a batch stays findable as a batch. A connector is the same
// question one level up: the operator chooses once, on the thing they set up,
// and every contact that connector goes on to create is filed under it.
//
// The other half of "which records came in from this source, in this period" is
// the record's own date and needs no word at all — which is why there is one
// control here and not two.

type CaptureConnection = components["schemas"]["CaptureConnection"];

/** The word this connector files under, or the empty string for none. */
export function ConnectorContextTagRow({
  conn,
}: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const vocabulary = useTagVocabulary();
  const save = useSetConnectorContextTag(conn.provider);
  const words = vocabulary.data?.tags ?? [];
  const chosen = conn.context_tag;

  // No vocabulary, or none this reader may see. A dial whose only option is
  // "none" asks a question with one answer — and unlike the import's, this row
  // sits among settings a reader is scanning rather than in a flow they are
  // walking, so an inert control here is pure noise.
  if (words.length === 0 && chosen === undefined) {
    return null;
  }

  return (
    <SettingRow
      testId={`connector-${conn.provider}-context-tag`}
      label={t("connectors.contextTag.label")}
      description={
        chosen?.archived
          ? // The word was retired after it was chosen, so nothing is being
            // filed. Said here because the alternative is a connector that
            // quietly stopped and an operator with no way to see why.
            t("connectors.contextTag.archived", { name: chosen.name ?? "" })
          : vocabulary.data?.truncated
            ? `${t("connectors.contextTag.hint")} ${t("tags.catalogTruncated")}`
            : t("connectors.contextTag.hint")
      }
      control={
        <Select
          aria-label={t("connectors.contextTag.label")}
          value={chosen?.id ?? ""}
          disabled={save.isPending}
          onChange={(next) => save.mutate(next === "" ? null : next)}
          options={[
            { value: "", label: t("connectors.contextTag.none") },
            ...words.map((word) => ({ value: word.id, label: word.name })),
          ]}
        />
      }
    />
  );
}

function useSetConnectorContextTag(provider: CaptureConnection["provider"]) {
  const queryClient = useQueryClient();
  return useMutation({
    // Takes the new word as a VARIABLE rather than closing over the row's
    // current one: the change belongs to the committed render, and a mutationFn
    // reading render state would answer with whatever the previous render held
    // (frontend/AGENTS.md, mutation-variable-coverage).
    mutationFn: async (tagID: string | null) => {
      const { error } = await api.PUT("/connectors/{provider}/context-tag", {
        params: { path: { provider } },
        body: { tag_id: tagID },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["connectors"] });
    },
  });
}
