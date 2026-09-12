import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState } from "react";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import { Button, Field, Modal, Textarea } from "../../design-system/atoms";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { dealRecordKeys } from "../activitykeys";
import { problemMessageOf, throwProblem } from "../common";

/** What the column takes (deal.description, length <= 20000). */
const BRIEF_MAX = 20000;

/**
 * Writing the deal's brief, from the panel that shows it.
 *
 * A focused PATCH of one field rather than a trip through the record's whole
 * edit form. The brief is prose somebody sits down to write, and the fifteen-
 * field form is not where they are when they think of it.
 *
 * The write is version-pinned like every other deal write. An unpinned save
 * would land on top of an edit it never saw and report success to both
 * authors, which for a field two people might both be rewriting is the exact
 * loss worth refusing.
 */
export function DealBriefEdit({
  open,
  onClose,
  dealId,
  version,
  brief,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  dealId: string;
  // The version the panel READ. It pins the save, so an edit that landed
  // while this modal sat open comes back as a conflict rather than silently
  // overwriting somebody's words.
  version: number | undefined;
  brief?: string | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const headingId = useId();
  const [text, setText] = useState("");
  const save = useSaveBrief(dealId);
  // Pulled out because the effect below depends on THIS function rather than
  // on the mutation object, which is a new object every render: depending on
  // the object would reset the form under the reader mid-typing.
  const resetSave = save.reset;

  // Re-seeded on every opening, not once at mount. The modal stays mounted for
  // the life of the panel, so an initializer would hand back whatever the
  // brief said the first time the page rendered.
  useEffect(() => {
    if (open) {
      setText(brief ?? "");
      resetSave();
    }
  }, [open, brief, resetSave]);

  const trimmed = text.trim();
  const tooLong = text.length > BRIEF_MAX;
  // Clearing the brief is a real edit, so an empty box saves null rather than
  // being refused. Only an unchanged box has nothing to send.
  const unchanged = trimmed === (brief ?? "").trim();

  async function submit() {
    if (tooLong || unchanged) {
      return;
    }
    await save.mutateAsync({
      version,
      description: trimmed === "" ? null : trimmed,
    });
    onClose();
  }

  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId}>
      <h2
        id={headingId}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {t("deal.briefEditTitle")}
      </h2>
      <div className="form-stack">
        <Field label={t("deal.brief")} hint={t("deal.briefHint")}>
          {(control) => (
            <Textarea
              {...control}
              rows={8}
              value={text}
              disabled={save.isPending}
              onChange={(event) => setText(event.target.value)}
            />
          )}
        </Field>
        {tooLong && (
          <p className="t-caption" role="alert">
            {/* The cap is a QUANTITY, so it is grouped in the reader's own
                notation: a German reader reads 20.000 beside every other
                figure on the page, not 20000. */}
            {t("deal.briefTooLong", { max: formatNumber(BRIEF_MAX, locale) })}
          </p>
        )}
        {save.isError && (
          <p className="t-caption" role="alert">
            {problemMessageOf(save.error, t)}
          </p>
        )}
        <div className="actions">
          <Button variant="ghost" onClick={onClose} disabled={save.isPending}>
            {t("deals.cancel")}
          </Button>
          <Button
            onClick={() => void submit()}
            disabled={save.isPending || tooLong || unchanged}
          >
            {t("deal.briefSave")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

/**
 * The brief alone, as its own write.
 *
 * `description` is the only field in the body: a PATCH naming the whole record
 * would send back every value this modal never showed the reader, and overwrite
 * with them whatever somebody else changed in the meantime.
 */
function useSaveBrief(dealId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (args: {
      version: number | undefined;
      description: string | null;
    }) => {
      const { data, error } = await api.PATCH("/deals/{id}", {
        params: {
          path: { id: dealId },
          ...ifMatch(requireVersion(args.version)),
        },
        body: { description: args.description },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["deals"] });
      for (const queryKey of dealRecordKeys(dealId)) {
        qc.invalidateQueries({ queryKey });
      }
    },
  });
}
