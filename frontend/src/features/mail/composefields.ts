// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useRef, useState } from "react";
import type { components } from "../../api/schema";
import type { ProjectScope } from "../../design-system/projectpicker";
import { paragraphsFrom } from "../../design-system/richtext";
import { isLocale, type Locale } from "../../i18n/locale";
import type { CommunicationContext } from "../../lib/compose-context";
import { useViewerZone } from "../../lib/viewerzone";
import type { DraftProvenance, DraftUnavailable } from "./composedraftcall";
import {
  type DraftMarks,
  holdsServedWords,
  MarksMemory,
  oneKeystroke,
  plainText,
} from "./composedraftmarks";
import type { AddressLine } from "./composehead";
import type { SavedDraftFields } from "./composesaveddraftcalls";
import { type ScheduleFields, scheduleFields } from "./composeschedule";
import type { EditSeed } from "./schedulededit";

type DraftReason = components["schemas"]["AccountDraftReason"];

// The chosen deal and the server's report of what its draft read.
function useAccountGrounding(edit: EditSeed | undefined) {
  const [dealId, setDealId] = useState(edit?.dealId ?? "");
  const [reasoning, setReasoning] = useState<DraftReason[]>([]);
  // The server's scope report for the draft on screen, retired with the
  // reasons because it describes the same read.
  const [scope, setScope] = useState<ProjectScope | undefined>(undefined);
  return {
    // A deal is chosen beside the project, among what the message is about.
    // The words stay: they are the rep's to read again, and a draft still in
    // flight for the old deal is dropped when it lands.
    dealId,
    setDealId,
    grounding: { dealId },
    reasoning,
    setReasoning,
    scope,
    setScope,
  };
}

// One conversation's editable offer, as the composer parks it on a switch.
export type FieldsSnapshot = Readonly<{
  to: string[];
  cc: string[];
  bcc: string[];
  copiesOpen: boolean;
  subject: string;
  body: string;
  html: string;
  intent: string;
  draftedIntent: string;
  context: CommunicationContext | "";
  sendAt: string;
  sendSchedule?: ScheduleFields;
  provenance: DraftProvenance | null;
  draftRef: string | null;
  servedBody: string;
  dealId: string;
  reasoning: DraftReason[];
  scope: ProjectScope | undefined;
  language: Locale | null;
}>;

export function emptyFields(): FieldsSnapshot {
  return {
    to: [],
    cc: [],
    bcc: [],
    copiesOpen: false,
    subject: "",
    body: "",
    html: "",
    intent: "",
    draftedIntent: "",
    context: "",
    sendAt: "",
    provenance: null,
    draftRef: null,
    servedBody: "",
    dealId: "",
    reasoning: [],
    scope: undefined,
    language: null,
  };
}

// What an edit does to the marks on screen: none when they stand. A body held
// before takes the marks it had; an emptied one sets them aside.
function marksAfterEdit(
  edit: Readonly<{
    text: string;
    marks: DraftMarks | null;
    remembered: DraftMarks | null | undefined;
    aside: DraftMarks | null;
    served: string;
  }>,
): Readonly<{ apply: DraftMarks | null }> | null {
  if (edit.remembered !== undefined) return { apply: edit.remembered };
  if (plainText(edit.text) === "") return edit.marks ? { apply: null } : null;
  const back =
    !edit.marks && edit.aside && holdsServedWords(edit.text, edit.served);
  return back ? { apply: edit.aside } : null;
}

const NO_REFUSALS: Readonly<Record<AddressLine, readonly string[]>> = {
  to: [],
  cc: [],
  bcc: [],
};

// Whether a seed arrives holding a copy, which opens the copy rows.
function holdsCopies(edit: EditSeed | undefined): boolean {
  return Boolean(edit && edit.cc.length + edit.bcc.length > 0);
}

// The message being written: its fields, the draft that filled them, and
// the epoch that retires a draft request once the words move on.
export function useComposeFields({
  initialMessage,
  initialReason,
  askedIntent,
  edit,
}: Readonly<{
  initialMessage?: Readonly<{ subject: string; body: string }>;
  initialReason?: CommunicationContext;
  askedIntent?: string;
  /** A scheduled message being written again; read once, when the composer mounts. */
  edit?: EditSeed;
}>) {
  const [to, setTo] = useState<string[]>(edit?.to ?? []);
  const [cc, setCc] = useState<string[]>(edit?.cc ?? []);
  // Copy rows holding a value may not be hidden, so they stay open once opened.
  const [bcc, setBcc] = useState<string[]>(edit?.bcc ?? []);
  const [copiesOpen, setCopiesOpen] = useState(() => holdsCopies(edit));
  const [subject, setSubject] = useState(
    edit?.subject ?? initialMessage?.subject ?? "",
  );
  // The plain part every gate reads, and the markup part beside it. Both
  // travel, because the wire is multipart/alternative.
  const [body, setBody] = useState(edit?.body ?? initialMessage?.body ?? "");
  // A seeded message arrives as plain text, and the editor draws the markup.
  const [html, setHtml] = useState(
    () =>
      edit?.html ?? (initialMessage ? paragraphsFrom(initialMessage.body) : ""),
  );
  const asked = askedIntent ?? "";
  const [intent, setIntent] = useState(asked);
  // The purpose the words on screen were last drafted to. A rewrite verb keeps
  // it, since it rewrites the words without reading the purpose.
  const [draftedIntent, setDraftedIntent] = useState(asked);
  // Keyed on the caller's ask, so a second moment action replaces the reason
  // the first one seeded.
  const [seededIntent, setSeededIntent] = useState(asked);
  if (asked !== seededIntent) {
    setSeededIntent(asked);
    setIntent(asked);
  }
  // What this message is, when the record does not say. Empty until the
  // reader answers, and never asked on a reply (see contextFor).
  const [context, setContext] = useState<CommunicationContext | "">(
    edit?.context ?? initialReason ?? "",
  );
  // The send-later moment as datetime-local text in the rep's zone. It
  // becomes an instant at submit (see scheduleFields).
  const [sendAt, setSendAtValue] = useState(edit?.sendAt ?? "");
  const [sendSchedule, setSendSchedule] = useState<ScheduleFields | undefined>(
    edit?.sendSchedule,
  );
  const readerZone = useViewerZone();
  const scheduleZone =
    sendSchedule?.scheduled_intent?.timezone ??
    sendSchedule?.scheduled_tz ??
    readerZone;
  const setSendAt = useCallback(
    (local: string) => {
      setSendAtValue(local);
      setSendSchedule(scheduleFields(local, undefined, scheduleZone));
    },
    [scheduleZone],
  );
  const [provenance, setProvenance] = useState<DraftProvenance | null>(null);
  // The served draft the body came from, so the server can tell a sent draft
  // from a rewritten one. It may only name the text on screen.
  const [draftRef, setDraftRef] = useState<string | null>(
    edit?.draftRef || null,
  );
  // The words as the model last served them; a rewrite may replace only these.
  const [servedBody, setServedBody] = useState("");
  // The draft language the rep picked; null leaves it to the correspondence.
  const [language, setLanguage] = useState<Locale | null>(null);
  // Two non-error outcomes kept out of react-query's error channel, so the
  // form stays usable when the model or mailer is not configured (501).
  const [draftUnavailable, setDraftUnavailable] =
    useState<DraftUnavailable | null>(null);
  const [sendUnavailable, setSendUnavailable] = useState(false);
  // Text a recipient line could not read as an address. It is no recipient,
  // so the send waits until the reader corrects or removes it.
  const [refusedAddresses, setRefusedAddresses] = useState(NO_REFUSALS);
  const addressRefused = Object.values(refusedAddresses).some(
    (parts) => parts.length > 0,
  );
  // Bumped when a reset clears refusals, so every address box drops what it
  // holds; with nothing refused, typed text stays.
  const [addressEpoch, setAddressEpoch] = useState(0);
  const forgetRefusals = () => {
    if (!addressRefused) return;
    setRefusedAddresses(NO_REFUSALS);
    setAddressEpoch((epoch) => epoch + 1);
  };
  // A new number marks a body the reader may undo: only the model's words.
  const [undoableRevision, setUndoableRevision] = useState(0);
  const refuseAddresses = useCallback(
    (line: AddressLine, parts: readonly string[]) =>
      setRefusedAddresses((prev) => ({ ...prev, [line]: parts })),
    [],
  );
  const [draftKept, setDraftKept] = useState(false);
  const grounding = useAccountGrounding(edit);
  // A ref invalidates in-flight drafts at once, including React Query's
  // window before it adopts the latest render's callbacks.
  const draftEpoch = useRef(0);
  // The model's words can come back by undo, and never without their marks;
  // the reader's own words never come back with the model's.
  const memory = useRef(new MarksMemory());
  const parked = useRef<DraftMarks | null>(null);
  const forgetMarks = () => {
    memory.current.forget();
    parked.current = null;
  };
  const account = grounding;

  // A new transport changes what is being written. A subject or address left
  // standing would be one the channel send never uses.
  const clearForTransport = () => {
    draftEpoch.current += 1;
    forgetMarks();
    forgetRefusals();
    setSubject("");
    setBody("");
    setHtml("");
    setTo([]);
    setCc([]);
    setBcc([]);
    setDraftRef(null);
    setProvenance(null);
  };

  const marksNow = (): DraftMarks | null =>
    provenance || draftRef
      ? {
          draftRef,
          provenance,
          reasoning: account.reasoning,
          scope: account.scope,
          language,
        }
      : null;
  // An emptied body sets the marks aside, and the choice stays for the next draft.
  const applyMarks = (marks: DraftMarks | null) => {
    setDraftRef(marks?.draftRef ?? null);
    setProvenance(marks?.provenance ?? null);
    account.setReasoning(marks?.reasoning ?? []);
    account.setScope(marks?.scope);
    if (marks) setLanguage(marks.language);
  };

  const editBody = (next: Readonly<{ html: string; text: string }>) => {
    if (next.text !== body || next.html !== html) draftEpoch.current += 1;
    setBody(next.text);
    setHtml(next.html);
    const marks = marksNow();
    memory.current.remember(body, marks);
    const change = marksAfterEdit({
      text: next.text,
      marks,
      remembered: oneKeystroke(body, next.text)
        ? undefined
        : memory.current.marksOf(next.text),
      aside: parked.current,
      served: servedBody,
    });
    if (!change) return;
    if (marks) parked.current = marks;
    applyMarks(change.apply);
  };

  // The model answers in plain text, so the markup starts as its paragraphs.
  const adoptDraftedBody = (drafted: string) => {
    memory.current.remember(body, marksNow());
    parked.current = null;
    setUndoableRevision((revision) => revision + 1);
    setBody(drafted);
    setHtml(paragraphsFrom(drafted));
  };

  // A rejected draft's words and disclosure leave with the judgment. An undo
  // may still bring the words back, so the disclosure waits for them.
  const clearRejected = () => {
    const rejected: DraftMarks | null = provenance
      ? {
          draftRef: null,
          provenance,
          reasoning: [],
          scope: undefined,
          language,
        }
      : null;
    memory.current.remember(body, rejected);
    parked.current = rejected;
    setBody("");
    setHtml("");
    setProvenance(null);
  };

  const restoreSaved = (saved: SavedDraftFields) => {
    draftEpoch.current += 1;
    forgetMarks();
    forgetRefusals();
    setTo([...saved.to]);
    setCc([...saved.cc]);
    setBcc([...saved.bcc]);
    if (saved.cc.length + saved.bcc.length > 0) setCopiesOpen(true);
    setSubject(saved.subject);
    setBody(saved.body);
    setHtml(saved.html);
    setContext(saved.context);
    setSendAtValue(saved.sendAt);
    setSendSchedule(saved.sendSchedule);
    const state = saved.editorState;
    const savedIntent = state?.intent ?? "";
    setIntent(savedIntent);
    // The saved state does not carry the drafted purpose, so a restored
    // purpose counts as the one its words were drafted to.
    setDraftedIntent(savedIntent);
    setServedBody(state?.served_body ?? "");
    setLanguage(chosenLanguage(state?.language));
    setDraftRef(null);
    setProvenance(
      state?.ai_generated
        ? {
            ai_generated: true,
            ai_disclosure: state.ai_disclosure,
            voice_profile_version: state.voice_profile_version,
            voice_degraded: state.voice_degraded ?? false,
          }
        : null,
    );
  };

  const snapshot = (): FieldsSnapshot => ({
    to,
    cc,
    bcc,
    copiesOpen,
    subject,
    body,
    html,
    intent,
    draftedIntent,
    context,
    sendAt,
    sendSchedule,
    provenance,
    draftRef,
    servedBody,
    dealId: account.dealId,
    reasoning: account.reasoning,
    scope: account.scope,
    language,
  });

  const load = (next: FieldsSnapshot) => {
    grounding.setDealId(next.dealId);
    draftEpoch.current += 1;
    forgetMarks();
    forgetRefusals();
    setTo(next.to);
    setCc(next.cc);
    setBcc(next.bcc);
    setCopiesOpen(next.copiesOpen);
    setSubject(next.subject);
    setBody(next.body);
    setHtml(next.html);
    setIntent(next.intent);
    setDraftedIntent(next.draftedIntent);
    setContext(next.context);
    setSendAtValue(next.sendAt);
    setSendSchedule(next.sendSchedule);
    setProvenance(next.provenance);
    setDraftRef(next.draftRef);
    setServedBody(next.servedBody);
    account.setReasoning(next.reasoning);
    account.setScope(next.scope);
    setLanguage(next.language);
    setDraftUnavailable(null);
  };

  return {
    editorState: editorStateOf({ intent, provenance, servedBody, language }),
    to,
    setTo,
    cc,
    setCc,
    bcc,
    setBcc,
    copiesOpen,
    openCopies: () => setCopiesOpen(true),
    subject,
    setSubject,
    body,
    html,
    intent,
    setIntent,
    draftedIntent,
    setDraftedIntent,
    context,
    setContext,
    sendAt,
    sendSchedule,
    scheduleControl: { sendAt, timezone: scheduleZone },
    setSendAt,
    provenance,
    setProvenance,
    draftRef,
    setDraftRef,
    servedBody,
    setServedBody,
    language,
    setLanguage,
    draftUnavailable,
    setDraftUnavailable,
    sendUnavailable,
    setSendUnavailable,
    refuseAddresses,
    refusedAddresses,
    addressEpoch,
    undoableRevision,
    addressRefused,
    draftKept,
    setDraftKept,
    account,
    draftEpoch,
    clearForTransport,
    editBody,
    adoptDraftedBody,
    clearRejected,
    restoreSaved,
    snapshot,
    load,
  };
}

export type ComposeFields = ReturnType<typeof useComposeFields>;

// A saved code this build does not ship is no choice it can name again.
function chosenLanguage(saved: string | undefined): Locale | null {
  return saved !== undefined && isLocale(saved) ? saved : null;
}

function editorStateOf(
  fields: Pick<
    FieldsSnapshot,
    "intent" | "provenance" | "servedBody" | "language"
  >,
): components["schemas"]["MailDraftEditorState"] {
  return {
    intent: fields.intent,
    ai_generated: fields.provenance?.ai_generated ?? false,
    ai_disclosure: fields.provenance?.ai_disclosure ?? undefined,
    voice_profile_version:
      fields.provenance?.voice_profile_version ?? undefined,
    voice_degraded: fields.provenance?.voice_degraded ?? false,
    served_body: fields.servedBody,
    ...(fields.language ? { language: fields.language } : {}),
  };
}
