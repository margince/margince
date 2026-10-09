// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useRef, useState } from "react";
import type { components } from "../api/schema";
import type { ProjectScope } from "../design-system/projectpicker";
import { paragraphsFrom } from "../design-system/richtext";
import type { CommunicationContext } from "./compose-context";
import type {
  DraftProvenance,
  DraftUnavailable,
  Grounding,
} from "./composedraftcall";
import type { SavedDraftFields } from "./composesaveddraft";

type DraftReason = components["schemas"]["AccountDraftReason"];

// The account-started path's grounding: the recipient, the deal and the
// project a draft is written for, with what the server said it wrote from.
function useAccountGrounding(
  contactId: string | undefined,
  onGroundingChanged: () => void,
) {
  const [recipientId, setRecipientId] = useState(contactId ?? "");
  const [dealId, setDealId] = useState("");
  // The project scopes the draft's grounding and files the sent message.
  const [projectId, setProjectId] = useState("");
  const [reasoning, setReasoning] = useState<DraftReason[]>([]);
  // The server's scope report for the draft on screen, retired with the
  // reasons because it describes the same read.
  const [scope, setScope] = useState<ProjectScope | undefined>(undefined);
  // A new recipient or deal retires the draft written for the previous pair.
  // A re-draft could not repair it, because the fill leaves a full field alone.
  const reground = (apply: (next: string) => void) => (next: string) => {
    apply(next);
    setReasoning([]);
    setScope(undefined);
    onGroundingChanged();
  };
  return {
    recipientId,
    setRecipientId: reground(setRecipientId),
    dealId,
    setDealId: reground(setDealId),
    projectId,
    setProjectId: reground(setProjectId),
    grounding: { recipientId, dealId, projectId } satisfies Grounding,
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
  bccOpen: boolean;
  subject: string;
  body: string;
  html: string;
  intent: string;
  context: CommunicationContext | "";
  sendAt: string;
  provenance: DraftProvenance | null;
  draftRef: string | null;
  servedBody: string;
  reasoning: DraftReason[];
  scope: ProjectScope | undefined;
}>;

export function emptyFields(): FieldsSnapshot {
  return {
    to: [],
    cc: [],
    bcc: [],
    bccOpen: false,
    subject: "",
    body: "",
    html: "",
    intent: "",
    context: "",
    sendAt: "",
    provenance: null,
    draftRef: null,
    servedBody: "",
    reasoning: [],
    scope: undefined,
  };
}

// The message being written: its fields, the draft that filled them, and
// the epoch that retires a draft request once the words move on.
export function useComposeFields({
  initialMessage,
  askedIntent,
  contactId,
}: Readonly<{
  initialMessage?: Readonly<{ subject: string; body: string }>;
  askedIntent?: string;
  contactId?: string;
}>) {
  const [to, setTo] = useState<string[]>([]);
  const [cc, setCc] = useState<string[]>([]);
  // A Bcc row holding a value may not be hidden, so it stays open once opened.
  const [bcc, setBcc] = useState<string[]>([]);
  const [bccOpen, setBccOpen] = useState(false);
  const [subject, setSubject] = useState(initialMessage?.subject ?? "");
  // The plain part every gate reads, and the markup part beside it. Both
  // travel, because the wire is multipart/alternative.
  const [body, setBody] = useState(initialMessage?.body ?? "");
  const [html, setHtml] = useState("");
  const [intent, setIntent] = useState(askedIntent ?? "");
  // Keyed on the caller's ask, so a second moment action replaces the reason
  // the first one seeded.
  const [seededIntent, setSeededIntent] = useState(askedIntent ?? "");
  if ((askedIntent ?? "") !== seededIntent) {
    setSeededIntent(askedIntent ?? "");
    setIntent(askedIntent ?? "");
  }
  // What this message is, when the record does not say. Empty until the
  // reader answers, and never asked on a reply (see contextFor).
  const [context, setContext] = useState<CommunicationContext | "">("");
  // The send-later moment as datetime-local text in the rep's zone. It
  // becomes an instant at submit (see scheduleFields).
  const [sendAt, setSendAt] = useState("");
  const [provenance, setProvenance] = useState<DraftProvenance | null>(null);
  // The served draft the body came from, so the server can tell a sent draft
  // from a rewritten one. It may only name the text on screen.
  const [draftRef, setDraftRef] = useState<string | null>(null);
  // The words as the model last served them; a rewrite may replace only these.
  const [servedBody, setServedBody] = useState("");
  // Two non-error outcomes kept out of react-query's error channel, so the
  // form stays usable when the model or mailer is not configured (501).
  const [draftUnavailable, setDraftUnavailable] =
    useState<DraftUnavailable | null>(null);
  const [sendUnavailable, setSendUnavailable] = useState(false);
  const [draftKept, setDraftKept] = useState(false);
  // A new grounding clears drafted text alone. `draftRef` and `provenance`
  // are set by the fill, so a rep's own typed message stays.
  const account = useAccountGrounding(contactId, () => {
    if (!provenance && !draftRef) {
      return;
    }
    setBody("");
    setHtml("");
    setSubject("");
    setTo([]);
    setDraftRef(null);
    setProvenance(null);
  });
  // A ref invalidates in-flight drafts at once, including React Query's
  // window before it adopts the latest render's callbacks.
  const draftEpoch = useRef(0);

  // A new transport changes what is being written. A subject or address left
  // standing would be one the channel send never uses.
  const clearForTransport = () => {
    draftEpoch.current += 1;
    setSubject("");
    setBody("");
    setHtml("");
    setTo([]);
    setCc([]);
    setBcc([]);
    setDraftRef(null);
    setProvenance(null);
  };

  // An emptied body no longer holds the served draft, so the reference, the
  // disclosure and the reasons go with it.
  const editBody = (next: Readonly<{ html: string; text: string }>) => {
    if (next.text !== body || next.html !== html) draftEpoch.current += 1;
    setBody(next.text);
    setHtml(next.html);
    if (next.text.trim()) {
      return;
    }
    setDraftRef(null);
    setProvenance(null);
    account.setReasoning([]);
    account.setScope(undefined);
  };

  // The model answers in plain text, so the markup starts as its paragraphs.
  const adoptDraftedBody = (drafted: string) => {
    setBody(drafted);
    setHtml(paragraphsFrom(drafted));
  };

  // A rejected draft's words and disclosure leave with the judgment.
  const clearRejected = () => {
    setBody("");
    setHtml("");
    setProvenance(null);
  };

  const restoreSaved = useCallback((saved: SavedDraftFields) => {
    draftEpoch.current += 1;
    setTo([...saved.to]);
    setCc([...saved.cc]);
    setBcc([...saved.bcc]);
    if (saved.bcc.length > 0) setBccOpen(true);
    setSubject(saved.subject);
    setBody(saved.body);
    setHtml(saved.html);
    setDraftRef(null);
    setProvenance(null);
  }, []);

  const snapshot = (): FieldsSnapshot => ({
    to,
    cc,
    bcc,
    bccOpen,
    subject,
    body,
    html,
    intent,
    context,
    sendAt,
    provenance,
    draftRef,
    servedBody,
    reasoning: account.reasoning,
    scope: account.scope,
  });

  const load = (next: FieldsSnapshot) => {
    setTo(next.to);
    setCc(next.cc);
    setBcc(next.bcc);
    setBccOpen(next.bccOpen);
    setSubject(next.subject);
    setBody(next.body);
    setHtml(next.html);
    setIntent(next.intent);
    setContext(next.context);
    setSendAt(next.sendAt);
    setProvenance(next.provenance);
    setDraftRef(next.draftRef);
    setServedBody(next.servedBody);
    account.setReasoning(next.reasoning);
    account.setScope(next.scope);
    setDraftUnavailable(null);
  };

  return {
    to,
    setTo,
    cc,
    setCc,
    bcc,
    setBcc,
    bccOpen,
    openBcc: () => setBccOpen(true),
    subject,
    setSubject,
    body,
    html,
    intent,
    setIntent,
    context,
    setContext,
    sendAt,
    setSendAt,
    provenance,
    setProvenance,
    draftRef,
    setDraftRef,
    servedBody,
    setServedBody,
    draftUnavailable,
    setDraftUnavailable,
    sendUnavailable,
    setSendUnavailable,
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
