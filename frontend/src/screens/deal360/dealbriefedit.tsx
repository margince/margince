import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
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
 * authors, which for a field two colleagues might both be rewriting is the
 * exact loss worth refusing.
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
  // The deal's LIVE version, which moves while this modal is open: the record
  // page refetches itself every LIVE_RECORD_MS. What pins the save is the
  // reading the modal opened on, captured below — see openedVersion.
  version: number | undefined;
  brief?: string | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const headingId = useId();
  const [text, setText] = useState("");
  // The brief as it was when this modal OPENED. The deal query refetches in
  // the background, so `brief` changes under a reader who is mid-sentence;
  // everything the form compares against has to be the reading it opened on,
  // or a colleague saving elsewhere silently rewrites what is being typed.
  const [opened, setOpened] = useState<string>("");
  // The version this modal OPENED on, and the reason it is not the `version`
  // prop: that one is the live deal's, and the record page re-reads itself
  // every sixty seconds (app/queryclient.ts, LIVE_RECORD_MS). Pinning the save
  // to the live reading makes If-Match agree with a version the FORM never
  // saw, so a colleague's rewrite that landed while somebody was typing is
  // answered 200 and silently replaced — the exact loss this pin exists to
  // refuse, and it only appeared once the page had re-read.
  //
  // The sibling editor beside this one pins the same way. Two forms over one
  // record cannot disagree about which reading a conflict is judged against.
  const [openedVersion, setOpenedVersion] = useState<number | undefined>();
  const save = useSaveBrief(dealId);
  // Pulled out because the effect below depends on THIS function rather than
  // on the mutation object, which is a new object every render: depending on
  // the object would reset the form under the reader mid-typing.
  const resetSave = save.reset;
  // The live brief, readable from the effect WITHOUT the effect depending on
  // it. Depending on it is what let a background refetch re-seed the textarea
  // and wipe an unsaved draft; reading it through a ref seeds from whatever is
  // current at the moment of opening and never again.
  const briefRef = useRef(brief);
  briefRef.current = brief;
  // The version travels the same way and for the same reason: read at the
  // moment of opening, never depended on, so a refetch cannot move it.
  const versionRef = useRef(version);
  versionRef.current = version;

  // Seeded on the OPENING edge alone. The modal stays mounted for the life of
  // the panel, so an initializer would hand back whatever the brief said the
  // first time the page rendered.
  useEffect(() => {
    if (open) {
      const current = briefRef.current ?? "";
      setText(current);
      setOpened(current);
      setOpenedVersion(versionRef.current);
      resetSave();
    }
  }, [open, resetSave]);

  const tooLong = text.length > BRIEF_MAX;
  // Compared RAW, against the reading this modal opened on. Trimming the
  // comparison would make an edit to the surrounding whitespace unsendable,
  // and comparing against the live `brief` would call a draft unchanged
  // because somebody else had just saved those same words.
  const unchanged = text === opened;

  async function submit() {
    if (tooLong || unchanged) {
      return;
    }
    await save.mutateAsync({
      // Pinned to the reading the FORM holds, not the live one. See
      // openedVersion.
      version: openedVersion,
      // Sent as typed. The server stores the description verbatim, so
      // trimming here would silently drop leading or trailing whitespace a
      // reader deliberately wrote — and make a whitespace-only correction
      // impossible. Only a box holding NOTHING is a cleared brief.
      description: text === "" ? null : text,
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
            onClick={() => {
              // The mutation already HOLDS the failure — the alert above renders
              // from its `isError`. What is swallowed here is only the promise
              // `mutateAsync` returns, which is otherwise an unhandled rejection
              // for a refusal the reader is already looking at.
              submit().catch(() => {});
            }}
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
