import { useCallback, useEffect, useId, useState } from "react";
import {
  Button,
  Checkbox,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import {
  type EmploymentActions,
  searchCompanyCandidates,
} from "./contactemployers";

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
}>) {
  const t = useT();
  const headingId = useId();
  const [company, setCompany] = useState<RecordPickerCandidate | null>(null);
  const [role, setRole] = useState("");
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
  // useState only reads its initial value ONCE, and this modal is mounted for
  // the life of the section rather than remounted per open. So the default has
  // to be re-taken every time it opens, or it answers a question about the rows
  // as they were the first time the section rendered: end the only employment,
  // reopen, and the box would still be unticked because the initializer had
  // already run — writing an explicit `false` for the contact's one current job,
  // which is the whole defect this default exists to prevent.
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
    setAllConnected(false);
    create.reset();
    onClose();
  }

  return (
    <Modal open={open} onClose={close} labelledBy={headingId}>
      <h2
        id={headingId}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {t("contact.rail.addEmployment")}
      </h2>
      <div className="form-stack">
        <div className="field">
          <span className="t-label">{t("contact.rail.employer")}</span>
          <RecordPicker
            label={t("contact.rail.employer")}
            searchTargets={searchTargets}
            selected={company}
            onPick={setCompany}
            disabled={create.isPending}
          />
          {!company && allConnected && (
            <p className="t-caption">
              {t("contact.rail.allCompaniesConnected")}
            </p>
          )}
        </div>
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
        <Checkbox
          label={t("contact.rail.isCurrentEmployer")}
          checked={isCurrent}
          disabled={create.isPending}
          onChange={(event) => setIsCurrent(event.target.checked)}
        />
      </div>
      {create.isError && (
        <p
          className="t-caption"
          role="alert"
          style={{ color: "var(--dangerText)" }}
        >
          {problemMessageOf(create.error, t)}
        </p>
      )}
      <div className="actions">
        <Button onClick={close} disabled={create.isPending}>
          {t("create.cancel")}
        </Button>
        <Button
          variant="primary"
          disabled={!company || create.isPending}
          onClick={() => {
            if (!company) {
              return;
            }
            create.mutate(
              {
                kind: "employment",
                contact_id: contactId,
                company_id: company.id,
                role: role.trim() || undefined,
                is_current_primary: isCurrent,
                // `manual` is the one word for a first-party write by a
                // contact — through this form or through an assistant. It used
                // to say "ui", which named the screen rather than the origin,
                // and would have re-created the spelling the backfill removed.
                source: "manual",
              },
              { onSuccess: close },
            );
          }}
        >
          {t("create.save")}
        </Button>
      </div>
    </Modal>
  );
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
