export {
  joinMultiselectValue,
  splitMultiselectValue,
} from "./create.multiselect";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { useIdempotencyKey } from "../api/idempotencykey";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import type { RequiredGrant } from "../lib/capability";
import { binding, shortcutTip } from "../lib/keymap";
import { leaveCreateForm } from "../lib/listreturn";
import { fromHere } from "../lib/navigatefrom";
import { ProblemError, problemExistingId } from "../lib/problem";
import { navigate, type Route, type Screen } from "../lib/router";
import { useShortcutAria } from "../lib/shortcuts";
import { useAddressWriters } from "../lib/urlstate";
import { useActionRefusal, useMeSettled } from "../lib/writerefusal";
import { catalogOfScreen, useSettledOpen } from "./create.dialog";
import "./create.css";
import "../design-system/querystates.css";

// The shared create-record form: each screen declares its fields and keeps its
// transport. A refusal stands on the field it names, in the reader's words.
import {
  type CreateField,
  type FormRows,
  keepUnsubmitted,
  usePublishedValues,
} from "./create.fields";
import { RecordFormBody } from "./create.form";
import { type CreateConflict, fieldRefusals } from "./create.refusals";

export { fieldControl } from "./create.controls";
export {
  answerless,
  type CreateField,
  type CreateFieldOption,
  dayOutOfRange,
  type FormRow,
  type FormRows,
  fieldLabel,
  fieldOptions,
  inlineTextKind,
  keepUnsubmitted,
  type SubField,
  submittedValues,
  usePublishedValues,
  visibleFields,
} from "./create.fields";
export { RecordFormBody, SUBMIT_COPY, type SubmitIntent } from "./create.form";
export type { CreateConflict } from "./create.refusals";

// The rail's WROTE head for a create here (agentrail-copy.ts). Only the three
// kinds a salesperson creates by hand have one; others leave the ticker silent.
const CREATE_MUTATION_HEAD: Readonly<Partial<Record<Screen, string>>> = {
  contacts: "contact-new",
  companies: "company-new",
  deals: "deal-new",
};

// A screen's create transport. `extra` carries body fields a refusal's own
// verb adds when it sends the same form again, such as the leads to convert.
export type CreateTransport<Created> = (
  values: Record<string, string>,
  rows: FormRows | undefined,
  idempotencyKey: string,
  extra?: Record<string, unknown>,
) => Promise<Created>;

// The shared post-create choreography: refresh the list, close the modal,
// open the fresh record's 360. Screens supply only their transport.
export function useCreateRecord<Created extends { id: string }>({
  create,
  invalidate,
  screen,
  onDone,
  stay = false,
  aboutId,
  onCreated,
}: Readonly<{
  create: CreateTransport<Created>;
  invalidate: string;
  screen: Screen;
  onDone: () => void;
  // The created record, for a caller that reports what landed. Runs before
  // onDone, so a caller can name the record in a toast while the form it came
  // from is still the thing on screen.
  onCreated?: (created: Created) => void;
  // For a create that is a PROPERTY of the record on screen (a tag, a list
  // membership): its id is not one `screen` can load, so the reader stays.
  stay?: boolean;
  // The record this create is ABOUT when not the one created: a new deal has no
  // id to name it by, so its company does. Absent, the ticker's plain phrase.
  aboutId?: string;
}>) {
  const queryClient = useQueryClient();
  const head = stay ? undefined : CREATE_MUTATION_HEAD[screen];
  return useMutation({
    mutationKey:
      head === undefined ? undefined : aboutId ? [head, aboutId] : [head],
    mutationFn: ({
      values,
      rows,
      idempotencyKey,
      extra,
    }: {
      values: Record<string, string>;
      rows: FormRows;
      idempotencyKey: string;
      extra?: Record<string, unknown>;
    }) => create(values, rows, idempotencyKey, extra),
    onMutate: fromHere,
    onSuccess: (created, _vars, from) => {
      // Asked before onDone, which may close the form's own address.
      const opens = !stay && from.stillHere();
      queryClient.invalidateQueries({ queryKey: [invalidate] });
      onCreated?.(created);
      onDone();
      if (opens) {
        navigate({ screen, id: created.id });
      }
    },
  });
}

// The whole per-screen create affordance in one piece: the button, the modal,
// its open state, and the post-create choreography. A list screen supplies
// its label, fields, and transport — nothing else.
export function CreateAction<Created extends { id: string }>({
  label,
  fields,
  create,
  invalidate,
  screen,
  listCreate,
  resolveExisting,
  stay = false,
  aboutId,
  keepOpen = false,
  onCreated,
  onValuesChange,
  testId,
  requires,
  refusedReason,
  refusalId,
  link,
  refusalNotice,
  conflict,
}: Readonly<{
  label: string;
  fields: CreateField[];
  // See CreateConflict.
  conflict?: CreateConflict;
  // Draws the verb as a card's text link. See NewRecordButton.
  link?: boolean;
  // The grant the create asks. Required, so no create opens a form its save
  // would refuse: a seat without it sees the verb refused with the reason. The
  // verb stays visible, because a hidden create reads as "there is nothing to
  // add".
  requires: RequiredGrant;
  // A refusal the caller knows beside the grant, with its own sentence; it
  // refuses the verb whatever the grant says.
  refusedReason?: string;
  // The id of a sentence already on the page saying why, for a header where
  // several verbs are refused by one fact.
  refusalId?: string;
  // The form's live answers, for a screen whose field list depends on them —
  // a dependent picker whose options only the server can narrow. See
  // usePublishedValues.
  onValuesChange?: (values: Record<string, string>) => void;
  // The key is one per open dialog and per record. A repeat of the same save
  // carries it, so a double press answers the first record. A POST that takes
  // no Idempotency-Key ignores it.
  create: CreateTransport<Created>;
  // A refusal this screen answers with its own verbs, or null for the
  // ordinary error line. `resend` sends the last submit again with `extra`.
  refusalNotice?: (
    error: unknown,
    resend: (extra: Record<string, unknown>) => void,
    pending: boolean,
  ) => ReactNode | null;
  invalidate: string;
  screen: Screen;
  // A list's form is its address `#/<screen>/new`, so closing it leaves that address.
  listCreate?: Readonly<{ open: boolean }>;
  // `keepOpen` makes one save "saved, next" for capture in a run: the modal
  // empties, not closes. It implies `stay`: opening the record would end a run.
  keepOpen?: boolean;
  // What was created, for a caller that reports it — the toast naming each
  // saved record is the only feedback a form that never closes gives.
  onCreated?: (created: Created) => void;
  // Names this button when a screen carries two of them. See NewRecordButton.
  testId?: string;
  // See useCreateRecord: keep the reader on this record when what was created
  // belongs TO it rather than being somewhere to go.
  stay?: boolean;
  // See useCreateRecord: the record this create is about, when it is not the
  // record being created.
  aboutId?: string;
  // Duplicate (409) dedupe: given the problem's code + collided record id,
  // builds the route to that record. Absent screens simply never show the
  // "view existing" link.
  resolveExisting?: (code: string, id: string) => Route;
}>) {
  const t = useT();
  const grantRefusal = useActionRefusal(requires);
  const refusal = refusedReason ?? grantRefusal;
  const refused = refusal !== undefined;
  const reason = refusalId === undefined ? refusal : undefined;
  // No form opens before /me has settled: what it says decides the refusal.
  const settled = useMeSettled();
  const [creating, setCreating] = useState(listCreate?.open ?? false);
  if (creating && refused) {
    setCreating(false);
  }
  const writers = useAddressWriters();
  // A refused create at its list's own address leaves it, as Cancel would,
  // so the address does not keep promising a form.
  const refusedAtAddress = refused && listCreate?.open === true;
  useEffect(() => {
    if (refusedAtAddress) {
      leaveCreateForm(screen, writers);
    }
  }, [refusedAtAddress, screen, writers]);
  const shown = useSettledOpen(creating, catalogOfScreen(screen));
  // Counts this session's saves, which empties the form between them. Not a
  // boolean: two saves in a row must read as two distinct clears.
  const [saved, setSaved] = useState(0);
  const { key: idempotencyKey, renew } = useIdempotencyKey();
  const mutation = useCreateRecord({
    create,
    invalidate,
    screen,
    stay: stay || keepOpen,
    aboutId,
    onDone: () => {
      renew();
      if (keepOpen) {
        setSaved((n) => n + 1);
        return;
      }
      close();
    },
    onCreated,
  });
  function close() {
    setCreating(false);
    if (listCreate) {
      leaveCreateForm(screen, writers);
    }
  }
  const existing =
    mutation.error instanceof ProblemError
      ? problemExistingId(mutation.error.problem)
      : null;
  const fieldRefused = fieldRefusals(
    mutation.error,
    fields,
    t,
    conflict,
    mutation.variables?.values,
  );
  const notice = mutation.isError
    ? refusalNotice?.(
        mutation.error,
        (extra) => {
          const last = mutation.variables;
          if (last) mutation.mutate({ ...last, extra });
        },
        mutation.isPending,
      )
    : null;
  return (
    <>
      <NewRecordButton
        label={label}
        onClick={() => {
          renew();
          // A refusal from an earlier opening is about words no longer there.
          mutation.reset();
          setCreating(true);
        }}
        testId={testId}
        pending={creating && !shown}
        reason={reason}
        reasonId={refused ? refusalId : undefined}
        link={link}
        opensOnCreateKey={listCreate !== undefined}
      />
      <CreateRecordModal
        // A refusal that lands while the dialog is open closes it. A verb the
        // reader may no longer take must not stay one press from the server.
        open={shown && settled && !refused}
        onClose={close}
        title={label}
        fields={fields}
        pending={mutation.isPending}
        error={
          mutation.isError && !fieldRefused.allPlaced ? fieldRefused.rest : null
        }
        fieldRefusals={fieldRefused.byField}
        refusedAt={mutation.isError ? mutation.submittedAt : undefined}
        existing={existing}
        resolveExisting={resolveExisting}
        notice={notice}
        resetToken={saved}
        onValuesChange={onValuesChange}
        onSubmit={(values, rows) =>
          mutation.mutate({ values, rows: rows ?? {}, idempotencyKey })
        }
      />
    </>
  );
}

export function NewRecordButton({
  label,
  onClick,
  testId = "new-record",
  pending,
  reason,
  reasonId,
  link,
  opensOnCreateKey = false,
}: Readonly<{
  label: string;
  onClick: () => void;
  // A record page card's header link ("New deal"): text only, no plus glyph.
  link?: boolean;
  // Two create buttons can share a list header (full form, quick capture); a
  // shared id would make both unaddressable.
  testId?: string;
  pending?: boolean;
  reason?: string;
  reasonId?: string;
  opensOnCreateKey?: boolean;
}>) {
  const createKey = useShortcutAria("create");
  const t = useT();
  const keyShortcut = opensOnCreateKey ? createKey : undefined;
  return (
    <Button
      onClick={onClick}
      data-testid={testId}
      pending={pending}
      reason={reason}
      reasonId={reasonId}
      variant={link ? "link" : undefined}
      aria-keyshortcuts={keyShortcut}
      title={
        keyShortcut === undefined
          ? undefined
          : shortcutTip(
              label,
              binding("create").keys,
              t("shell.shortcuts.then"),
            )
      }
    >
      {link ? (
        label
      ) : (
        <>
          <Plus aria-hidden /> {label}
        </>
      )}
    </Button>
  );
}

export function CreateRecordModal({
  open,
  onClose,
  title,
  fields,
  pending,
  error,
  fieldRefusals,
  refusedAt,
  existing,
  resolveExisting,
  notice,
  onSubmit,
  onValuesChange,
  resetToken = 0,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  title: string;
  fields: CreateField[];
  pending: boolean;
  error: string | null;
  fieldRefusals?: ReadonlyMap<string, string>;
  refusedAt?: number;
  existing?: { id: string; code: string } | null;
  resolveExisting?: (code: string, id: string) => Route;
  notice?: ReactNode;
  onSubmit: (values: Record<string, string>, rows?: FormRows) => void;
  // The form's live answers, for a caller driving a server read from them.
  onValuesChange?: (values: Record<string, string>) => void;
  // Bumped after a save that keeps the form open; the seeding below treats it
  // as a fresh open. A number, not a callback: the reset happens during render.
  resetToken?: number;
}>) {
  const [values, setValues] = useState<Record<string, string>>({});
  const [rows, setRows] = useState<FormRows>({});
  // What the last submit carried, so a reset can tell the saved record's words
  // apart from words typed after it while the save was still in flight.
  const [submitted, setSubmitted] = useState<Record<string, string>>({});
  // Seeded DURING RENDER on the closed→open transition, not in an effect (see
  // EditRecordModal), nor on `fields`, which a refetch renews, wiping input.
  // Starts false, not `open`: a modal mounted already open still has to seed.
  const [seededOpen, setSeededOpen] = useState(false);
  const [seededReset, setSeededReset] = useState(resetToken);
  usePublishedValues(values, onValuesChange);
  const reopened = open !== seededOpen;
  const cleared = open && resetToken !== seededReset;
  if (reopened || cleared) {
    setSeededOpen(open);
    setSeededReset(resetToken);
    if (open) {
      // A fresh open starts from the fields' defaults (first select option
      // for required selects), never from a previous attempt's leftovers.
      const defaults: Record<string, string> = {};
      for (const field of fields) {
        if (field.type === "select" && field.required) {
          defaults[field.key] = field.options?.[0]?.value ?? "";
        }
      }
      // A form kept open clears only what the save carried: words typed while
      // it was in flight belong to the next record.
      setValues((current) =>
        reopened ? defaults : keepUnsubmitted(current, submitted, defaults),
      );
      setRows({});
    }
  }

  return (
    <RecordFormBody
      dialog={{ open, title }}
      fields={fields}
      values={values}
      setValues={setValues}
      rows={rows}
      setRows={setRows}
      pending={pending}
      error={error}
      fieldRefusals={fieldRefusals}
      refusedAt={refusedAt}
      existing={existing}
      resolveExisting={resolveExisting}
      notice={notice}
      onSubmit={(sent, sentRows) => {
        setSubmitted(sent);
        onSubmit(sent, sentRows);
      }}
      onClose={onClose}
      intent="create"
    />
  );
}
