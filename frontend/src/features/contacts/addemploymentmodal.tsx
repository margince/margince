import { useCallback, useEffect, useId, useMemo, useState } from "react";
import {
  Button,
  Checkbox,
  Field,
  Modal,
  TextInput,
} from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import { stable } from "../../format/collate";
import { useT } from "../../i18n";
import {
  type Employment,
  type EmploymentActions,
  searchCompanyCandidates,
} from "./contactemployers";
import { stillHeld } from "./employmentcurrency";
import { datePatch, validDateEntry } from "./employmentpatch";
import "../../design-system/querystates.css";

// The "add a company" modal: pick the company (RecordPicker, the shared
// debounced search-and-pick), optionally its role, and whether it is the
// current primary employer — a Checkbox, not a Switch, because ticking it
// states an intent this modal's own Save then writes, it is not itself the
// write (design-system/README.md's Checkbox/Switch distinction).
export function AddEmploymentModal({
  open,
  onClose,
  contactId,
  create,
  excludedCompanyIds,
  hasCurrentEmployment,
  initialTerm,
  initialCompany = null,
  onLinked,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  contactId: string;
  create: EmploymentActions["create"];
  // Companies this contact already has a live employment edge to — the
  // picker refuses to offer a second edge to the same company, since only
  // a duplicated current-primary is refused server-side.
  excludedCompanyIds: ReadonlyArray<string>;
  // Whether this contact already holds a job that has not ended. It is the exact
  // fact the server's own rule turns on, read off the same rows, so the box can
  // START in the state the save will produce instead of showing the reader one
  // answer and writing the other.
  hasCurrentEmployment: boolean;
  // A name to start the search on, or a company already chosen. Read at mount.
  initialTerm?: string;
  initialCompany?: RecordPickerCandidate | null;
  // Runs once the employment is written, before the modal closes.
  onLinked?: () => void;
}>) {
  const t = useT();
  const headingId = useId();
  const [company, setCompany] = useState<RecordPickerCandidate | null>(
    initialCompany,
  );
  const [role, setRole] = useState("");
  // A FIRST-CLASS field rather than something to correct afterwards. An
  // employment with no start date has no window, and the account walk that
  // resolves old mail then reads a message from three jobs ago as belonging to
  // the company this contact joined last month. This form is where a human can
  // say when it began, and it is the only place that ever asks.
  //
  // Optional, and empty stays empty: "I do not know when" is a real answer, and
  // a form that defaulted to today would write a fact nobody has.
  const [start, setStart] = useState("");
  // Ticked by default for somebody with no current job, because that is what
  // the save will do either way: the server marks a contact's only current
  // employment as their primary one. A box that started unticked and then
  // produced the opposite was worse than sending the wrong value — it showed
  // the reader a state the record never took, and left "no" expressible only by
  // ticking and unticking again.
  //
  // So the box always STATES an answer and the reader can change it. The
  // server's rule still exists for callers who send nothing — MCP and the API —
  // and `hasCurrentEmployment` is read off the same rows that rule reads, so the
  // two agree by construction rather than by being maintained in step.
  const [isCurrent, setIsCurrent] = useState(!hasCurrentEmployment);
  // A caller may keep this modal mounted across opens, so each open takes the
  // default again.
  useEffect(() => {
    if (open) {
      setIsCurrent(!hasCurrentEmployment);
    }
  }, [open, hasCurrentEmployment]);
  const [allConnected, setAllConnected] = useState(false);

  // Wraps the shared company search with this contact's own already-connected
  // list. Kept on `excludedCompanyIds` alone, nothing that changes while the
  // reader types — RecordPicker treats a new `searchTargets` identity as a
  // new search space and empties whatever it was already showing.
  const searchTargets = useCallback(
    async (q: string) => {
      const results = await searchCompanyCandidates(q);
      const offered = results.filter(
        (candidate) => !excludedCompanyIds.includes(candidate.id),
      );
      // Every match this query found is a company already on the list, not
      // an empty search — the two read the same in a bare candidate box, so
      // the modal says which one it is rather than leaving a silent gap.
      setAllConnected(results.length > 0 && offered.length === 0);
      return offered;
    },
    [excludedCompanyIds],
  );

  function close() {
    setCompany(null);
    setRole("");
    // A caller may keep the modal mounted, so each field is put back by hand:
    // a start date left standing would date the next employer.
    setStart("");
    setAllConnected(false);
    create.reset();
    onClose();
  }

  return (
    <Modal open={open} onClose={close} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("contact.rail.addEmployment")}
      </Heading>
      <div className="form-stack">
        <Field label={t("contact.rail.employer")}>
          {(control) => (
            <>
              <RecordPicker
                id={control.id}
                aria-describedby={control["aria-describedby"]}
                searchTargets={searchTargets}
                selected={company}
                onPick={setCompany}
                initialTerm={initialTerm}
                disabled={create.isPending}
              />
              {!company && allConnected && (
                <p>{t("contact.rail.allCompaniesConnected")}</p>
              )}
            </>
          )}
        </Field>
        <Field label={t("rel.role")}>
          {(control) => (
            <TextInput
              {...control}
              value={role}
              disabled={create.isPending}
              onChange={(event) => setRole(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("employment.start")} hint={t("employment.dateHint")}>
          {(control) => (
            <TextInput
              {...control}
              value={start}
              disabled={create.isPending}
              onChange={(event) => setStart(event.target.value)}
            />
          )}
        </Field>
        <Checkbox
          label={t("contact.rail.isCurrentEmployer")}
          checked={isCurrent}
          disabled={create.isPending}
          onChange={(event) => setIsCurrent(event.target.checked)}
        />
        <ErrorLine error={create.error} />
      </div>
      <div className="actions">
        <Button onClick={close} disabled={create.isPending}>
          {t("create.cancel")}
        </Button>
        <Button
          variant="primary"
          disabled={!company || !validDateEntry(start)}
          pending={create.isPending}
          onClick={async () => {
            if (!company) {
              return;
            }
            const started = datePatch(start);
            // Awaited rather than a mutate callback: the link can replace the
            // row that holds this dialog, and an unmounted observer drops those.
            const linked = await create
              .mutateAsync({
                kind: "employment",
                contact_id: contactId,
                company_id: company.id,
                role: role.trim() || undefined,
                started_at: started.date,
                started_precision: started.precision,
                is_current_primary: isCurrent,
                // `manual` is the one word for a first-party write by a
                // contact — through this form or through an assistant. It used
                // to say "ui", which named the screen rather than the origin,
                // and would have re-created the spelling the backfill removed.
                source: "manual",
              })
              .then(
                () => true,
                // A refusal stays on screen through `create.error`.
                () => false,
              );
            if (linked) {
              onLinked?.();
              close();
            }
          }}
        >
          {t("create.save")}
        </Button>
      </div>
    </Modal>
  );
}

/** What the add dialog opens on: a name to search, or a company already chosen. */
export type EmploymentSeed = Readonly<{
  term?: string;
  company?: RecordPickerCandidate;
  onLinked?: () => void;
}>;

/**
 * The add dialog for one contact, and the verb that opens it. Each open
 * remounts the dialog, so its seed and defaults are read afresh.
 */
export function useAddEmployment(
  contactId: string,
  create: EmploymentActions["create"],
  employments: readonly Employment[],
) {
  const [adding, setAdding] = useState<
    EmploymentSeed & Readonly<{ open: boolean; seq: number }>
  >({ open: false, seq: 0 });
  // A string key, so the picker keeps its candidates until the set of ids
  // changes; `stable` because the key is only compared with itself.
  const connectedKey = employments
    .map((employment) => employment.company_id)
    .sort(stable)
    .join(",");
  const connected = useMemo(
    () => (connectedKey === "" ? [] : connectedKey.split(",")),
    [connectedKey],
  );
  const modal = (
    <AddEmploymentModal
      key={adding.seq}
      open={adding.open}
      onClose={() => setAdding((prior) => ({ ...prior, open: false }))}
      initialTerm={adding.company ? undefined : adding.term}
      initialCompany={adding.company}
      onLinked={adding.onLinked}
      contactId={contactId}
      create={create}
      excludedCompanyIds={connected}
      hasCurrentEmployment={employments.some(stillHeld)}
    />
  );
  return {
    modal,
    open: (seed: EmploymentSeed = {}) =>
      setAdding((prior) => ({ ...seed, open: true, seq: prior.seq + 1 })),
  };
}

// The date range only — role is now its own InlineText control above, so
// repeating it here would be the same fact twice. Through `formatDayMonth`,
// which is the same function every other date on this page goes through — an
// earlier version of this comment claimed the two sections could not disagree
// about what "12 Jan" means while each held its own private copy of the
// rendering, and both read the browser's guessed locale rather than the
// reader's chosen one.
// An employment that has ENDED says so even when nobody recorded when it
// began: a period is a nicety, but a former employer that reads like a current
// one is a rep writing to the wrong company. Only a connection with neither
// date has nothing to say.
