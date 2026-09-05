import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, LogOut, Mail } from "lucide-react";
import { type FormEvent, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { forgetHashCredential, takeHashCredential } from "../app/router";
import { Button, EmptyState, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Eyebrow } from "../design-system/eyebrow";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { Wordmark } from "./auth";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import { DOCUMENT_GROUPS } from "./dealroomdocuments";
import { type BoardDocument, DocumentBoard } from "./dealroomthreads";
import { downloadBytes } from "./download";
import "./buyerroom.css";

// The Deal Room as its BUYER sees it — the one screen an outside person ever
// reaches in this app. Anonymous: no seat, no cookie. The invitation link lands
// on `#/room?c=<credential>`; the credential comes out of the address bar as the
// router reads the hash (app/router.tsx's takeHashCredential, which is ahead of
// every gate that can render instead of this screen), and this screen takes it
// from there and exchanges it for a room session the tab keeps in
// sessionStorage and presents as a Bearer on every call. A dead link, a paused
// room and an expired one each get their own honest screen and their own way
// back, and none of them names anything the link did not already name.

type BuyerRoomView = components["schemas"]["BuyerRoomView"];

const SESSION_KEY = "margince.room.session";
const ROOM_ROUTE = "room";

function readSession(): string | null {
  try {
    return globalThis.sessionStorage?.getItem(SESSION_KEY) ?? null;
  } catch {
    return null;
  }
}

function writeSession(token: string | null): void {
  try {
    if (token === null) {
      globalThis.sessionStorage?.removeItem(SESSION_KEY);
    } else {
      globalThis.sessionStorage?.setItem(SESSION_KEY, token);
    }
  } catch {
    // A browser refusing storage still gets this one page view: the token
    // lives in React state for the tab's lifetime and is simply not kept.
  }
}

function bearer(token: string): { headers: { Authorization: string } } {
  return { headers: { Authorization: `Bearer ${token}` } };
}

// The session stopped answering — revoked, lapsed, or signed out elsewhere.
class SessionRefusedError extends Error {}

// refuseOrThrow turns a failed public call into the right error: a 401 is the
// session ending (the caller retires it), anything else is the server's own
// explanation. Every buyer write goes through it so none can keep a dead token.
function refuseOrThrow(
  error: unknown,
  response: Response,
  t: ReturnType<typeof useT>,
): never {
  if (response.status === 401) {
    throw new SessionRefusedError();
  }
  throwProblem(error, t);
  throw new Error("unreachable");
}

function retireOnRefusal(onSessionLost: () => void) {
  return (error: unknown) => {
    if (error instanceof SessionRefusedError) {
      onSessionLost();
    }
  };
}

export function BuyerRoomScreen() {
  // Read at mount AND whenever the address changes to carry a new one.
  //
  // A SECOND link pasted into a tab already sitting on #/room changes only the
  // hash, which React does not treat as a new mount. Reading once meant that
  // link was ignored and the tab went on presenting whatever session it already
  // held, including a dead one: the buyer sees "Nothing published yet" for a
  // room that has published, and concludes the link is broken.
  const [credential, setCredential] = useState(() =>
    takeHashCredential(ROOM_ROUTE),
  );
  useEffect(() => {
    const onHashChange = () => {
      const next = takeHashCredential(ROOM_ROUTE);
      if (next) {
        setCredential(next);
      }
    };
    globalThis.addEventListener?.("hashchange", onHashChange);
    return () => globalThis.removeEventListener?.("hashchange", onHashChange);
  }, []);
  // A link in hand outranks a kept session from the first render: a tab that
  // still holds room A's session must not show room A for a breath while
  // room B's link is being exchanged.
  const [token, setToken] = useState(() => (credential ? null : readSession()));
  // What the exchange answered, held HERE rather than read off the mutation:
  // the replayed mount (StrictMode) swaps the observer that ran it for one that
  // never hears the result, so isSuccess/isError would stay false for ever.
  const [refusal, setRefusal] = useState<Error | null>(null);
  const t = useT();

  const exchange = useMutation({
    mutationKey: ["buyer-room-exchange"],
    mutationFn: async (raw: string) => {
      const { data, error, response } = await api.POST(
        "/public/rooms/exchange",
        { body: { credential: raw } },
      );
      if (error) {
        if (response.status === 404) {
          throw new SessionRefusedError();
        }
        throwProblem(error, t);
      }
      return data;
    },
  });

  // A fresh link outranks a kept session: the person clicked it on purpose.
  // Exchanged at most ONCE per mount, held in a ref rather than in state:
  // the credential is single-use, and an effect that runs twice (StrictMode
  // replays mount effects in development) would consume it on the first run
  // and be refused on the second, showing a dead-link page for a live link.
  //
  // The token is taken from the promise rather than from an onSuccess option:
  // the replayed mount unsubscribes the first observer, and an option callback
  // on an observer nobody listens to never runs.
  const exchangeAsync = exchange.mutateAsync;
  // Every credential this tab has ALREADY spent, not merely the last one. A
  // link is single-use, so A → B → A must not send A a second time: the server
  // refuses the replay, and the refusal would then displace the working session
  // B had just opened.
  const spent = useRef(new Set<string>());
  // Which credential the tab is currently exchanging. A reply that arrives for
  // anything else is a superseded link answering late, and must not touch the
  // session — two links pasted in quick succession would otherwise race, and
  // whichever answered last would win regardless of which the person meant.
  const awaiting = useRef<string | null>(null);
  useEffect(() => {
    if (!credential || spent.current.has(credential)) {
      return;
    }
    spent.current.add(credential);
    // Out of the router's memory as well, which is where the address used to
    // hold it: this tab is spending it now, and a remount that found it there
    // would spend it again and be refused for a session that is working.
    forgetHashCredential(ROOM_ROUTE, credential);
    awaiting.current = credential;
    // The session the tab already holds is KEPT while the new link is checked.
    // Clearing it first showed the dead-link page over a room the person could
    // still read whenever the new link turned out to be expired — and a refresh
    // then brought that room back from storage, which is a different answer to
    // the same question a moment apart.
    setRefusal(null);
    exchangeAsync(credential).then(
      (issued) => {
        if (awaiting.current !== credential || !issued) {
          return;
        }
        awaiting.current = null;
        writeSession(issued.session_token);
        setToken(issued.session_token);
      },
      (error: unknown) => {
        if (awaiting.current !== credential) {
          return;
        }
        awaiting.current = null;
        setRefusal(error instanceof Error ? error : new Error(String(error)));
      },
    );
  }, [credential, exchangeAsync]);

  const signOut = () => {
    writeSession(null);
    setToken(null);
  };

  // "Opening" until the exchange has answered — not merely while the mutation
  // is in flight, because the first render happens before the effect fires it.
  if (credential && !token && !refusal) {
    return (
      <BuyerFrame>
        <EmptyState>{t("buyer.opening")}</EmptyState>
      </BuyerFrame>
    );
  }
  if (credential && refusal) {
    return (
      <BuyerFrame>
        <DeadLink
          message={
            refusal instanceof SessionRefusedError
              ? t("buyer.linkDead")
              : problemMessageOf(refusal, t)
          }
        />
      </BuyerFrame>
    );
  }
  if (!token) {
    return (
      <BuyerFrame>
        <DeadLink message={t("buyer.noLink")} />
      </BuyerFrame>
    );
  }
  return (
    <BuyerFrame>
      <RoomBody token={token} onSessionLost={signOut} />
    </BuyerFrame>
  );
}

function BuyerFrame({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <div className="buyer-page">
      <div className="buyer-column">{children}</div>
      <PoweredBy />
    </div>
  );
}

// The one thing on the buyer's page that is ours rather than the seller's:
// the product's mark, in the corner, saying what is serving the room.
function PoweredBy() {
  const t = useT();
  return (
    <span className="buyer-powered">
      <span className="t-small" aria-hidden>
        {t("buyer.poweredBy")}
      </span>
      <Wordmark
        alt={t("buyer.poweredByMargince")}
        className="buyer-powered-mark"
      />
    </span>
  );
}

// The link no longer admits anyone. Whatever the reason — used, lapsed,
// retired, never valid — the page says the same thing and offers the one
// recovery a buyer has: a fresh link to the address they were invited at.
function DeadLink({ message }: Readonly<{ message: string }>) {
  const t = useT();
  return (
    <Panel title={t("buyer.deadTitle")}>
      <PanelBody>
        <p>{message}</p>
      </PanelBody>
      <PanelBody>
        <LinkRequest />
      </PanelBody>
    </Panel>
  );
}

function LinkRequest() {
  const t = useT();
  const [email, setEmail] = useState("");
  const request = useMutation({
    mutationKey: ["buyer-room-link-request"],
    mutationFn: async (address: string) => {
      const { error } = await api.POST("/public/rooms/link-request", {
        body: { email: address },
      });
      if (error) {
        throwProblem(error, t);
      }
    },
  });
  if (request.isSuccess) {
    return (
      <Callout tone="success" live="status">
        {t("buyer.linkRequested")}
      </Callout>
    );
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    request.mutate(email.trim());
  };
  return (
    <form className="buyer-linkrequest" onSubmit={submit}>
      <Field label={t("buyer.emailLabel")} hint={t("buyer.emailHint")}>
        {(field) => (
          <TextInput
            {...field}
            type="email"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        )}
      </Field>
      <Button
        type="submit"
        variant="primary"
        pending={request.isPending}
        disabled={email.trim() === ""}
      >
        <Mail aria-hidden />
        {t("buyer.requestLink")}
      </Button>
      {request.isError ? (
        <p className="t-small t-danger">{problemMessageOf(request.error, t)}</p>
      ) : null}
    </form>
  );
}

function useBuyerRoom(token: string, onSessionLost: () => void) {
  const t = useT();
  const query = useQuery({
    queryKey: ["buyer-room", token],
    retry: false,
    // Re-asked whenever the tab comes back: a revocation or a pause made while
    // the buyer was away must bind on their return, not on their next click.
    refetchOnWindowFocus: "always",
    queryFn: async () => {
      const { data, error, response } = await api.GET("/public/rooms/me", {
        ...bearer(token),
      });
      if (error) {
        if (response.status === 401) {
          throw new SessionRefusedError();
        }
        throwProblem(error, t);
      }
      return data;
    },
  });
  const lost = query.error instanceof SessionRefusedError;
  useEffect(() => {
    if (lost) {
      onSessionLost();
    }
  }, [lost, onSessionLost]);
  return query;
}

function RoomBody({
  token,
  onSessionLost,
}: Readonly<{ token: string; onSessionLost: () => void }>) {
  const t = useT();
  const room = useBuyerRoom(token, onSessionLost);
  const signOut = useMutation({
    mutationKey: ["buyer-room-sign-out"],
    mutationFn: async (session: string) => {
      const { error } = await api.POST("/public/rooms/sign-out", {
        ...bearer(session),
      });
      if (error) {
        throwProblem(error, t);
      }
    },
    // Whatever the server said, this tab is done with the token.
    onSettled: onSessionLost,
  });
  return (
    <QueryStates
      query={room}
      pendingLines={4}
      pendingLabel={t("room.card.title")}
    >
      {room.data ? (
        <>
          <RoomView
            view={room.data}
            token={token}
            onSessionLost={onSessionLost}
          />
          <div className="buyer-foot">
            <Button
              variant="ghost"
              pending={signOut.isPending}
              onClick={() => signOut.mutate(token)}
            >
              <LogOut aria-hidden />
              {t("buyer.signOut")}
            </Button>
          </div>
        </>
      ) : null}
    </QueryStates>
  );
}

// The buyer's documents query, shared by the board and the decision verbs so
// one request serves both.
function useBuyerDocuments(token: string, onSessionLost: () => void) {
  const t = useT();
  const docs = useQuery({
    queryKey: ["buyer-room-documents", token],
    retry: false,
    // Re-asked with the tab, for the same reason /public/rooms/me is: a release
    // published while the buyer was away is what they came back to read.
    refetchOnWindowFocus: "always",
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/public/rooms/documents",
        { ...bearer(token) },
      );
      if (error) {
        if (response.status === 401) {
          throw new SessionRefusedError();
        }
        throwProblem(error, t);
      }
      return data;
    },
  });
  const lost = docs.error instanceof SessionRefusedError;
  useEffect(() => {
    if (lost) {
      onSessionLost();
    }
  }, [lost, onSessionLost]);
  return docs;
}

type BuyerRoomDocument = components["schemas"]["BuyerRoomDocument"];

// The buyer's one verb on a document: take a copy of it.
//
// There is no "confirm this version" and no "request changes". Sharing a
// document with a buyer is sharing it — asking them to formally accept each
// file turns a room into an approval queue nobody asked for, and the buyer
// reading "Confirm this version" under a transcript cannot tell what they
// would be agreeing to. Anything they want to say about a document they say in
// the thread under it, which is the whole point of the board.
function BuyerDocumentVerbs({
  token,
  doc,
}: Readonly<{ token: string; doc: BuyerRoomDocument }>) {
  const t = useT();
  const download = useMutation({
    mutationKey: ["buyer-room-document-download"],
    // The failure line rides as a mutation VARIABLE rather than being read off
    // `t` inside the function. A mutationFn is re-armed in a passive effect, so
    // a closure read here is the render-before-last's — for a translator that
    // means the locale the reader has just left.
    mutationFn: async (input: {
      documentId: string;
      filename: string;
      failure: string;
      token: string;
    }) => {
      const { data, error, response } = await api.GET(
        "/public/rooms/documents/{documentId}/file",
        {
          params: { path: { documentId: input.documentId } },
          parseAs: "blob",
          ...bearer(input.token),
        },
      );
      if (error || !data) {
        // A refusal this screen decided, with copy it already translated, so
        // it rides as a problem body — a plain Error is wording nobody wrote
        // for a user and is replaced by the shared failure line.
        throwProblem({ status: response.status, detail: input.failure });
      }
      // The blob's OWN type, because the server chose it: a PDF handed to the
      // reader as application/octet-stream downloads with the wrong icon and
      // opens in nothing.
      downloadBytes(data, input.filename, data.type);
    },
  });
  return (
    <div className="buyer-doc-actions">
      <Button
        small
        aria-label={t("buyer.docs.download", { title: doc.title })}
        pending={download.isPending}
        onClick={() =>
          download.mutate({
            documentId: doc.id,
            filename: doc.filename,
            failure: t("buyer.docs.downloadFailed"),
            token,
          })
        }
      >
        <Download aria-hidden />
        {t("buyer.docs.downloadShort")}
      </Button>
      {download.isError ? (
        <p className="t-small t-danger">
          {problemMessageOf(download.error, t)}
        </p>
      ) : null}
    </div>
  );
}

// The buyer's board: the shared documents, each with the threads about it,
// and the room-wide conversation. The verbs are the buyer's — download, open,
// reply — and never resolve.
function BuyerBoard({
  token,
  onSessionLost,
  mayWrite,
  refusal,
}: Readonly<{
  token: string;
  onSessionLost: () => void;
  mayWrite: boolean;
  refusal: string | undefined;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const docs = useBuyerDocuments(token, onSessionLost);
  const threads = useQuery({
    queryKey: ["buyer-room-threads", token],
    retry: false,
    // The conversation is live on both sides, so a returning tab re-reads it.
    refetchOnWindowFocus: "always",
    queryFn: async () => {
      const { data, error, response } = await api.GET("/public/rooms/threads", {
        ...bearer(token),
      });
      if (error) {
        if (response.status === 401) {
          throw new SessionRefusedError();
        }
        throwProblem(error, t);
      }
      return data;
    },
  });
  const lost = threads.error instanceof SessionRefusedError;
  useEffect(() => {
    if (lost) {
      onSessionLost();
    }
  }, [lost, onSessionLost]);
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ["buyer-room-threads", token] });
  const open = useMutation({
    mutationKey: ["buyer-room-thread-open"],
    mutationFn: async (input: {
      documentId: string | null;
      body: string;
      requiredChange: boolean;
    }) => {
      const { data, error, response } = await api.POST(
        "/public/rooms/threads",
        {
          body: {
            document_id: input.documentId,
            body: input.body,
            required_change: input.requiredChange,
          },
          ...bearer(token),
        },
      );
      if (error) {
        refuseOrThrow(error, response, t);
      }
      return data;
    },
    onError: retireOnRefusal(onSessionLost),
    onSuccess: refresh,
  });
  const reply = useMutation({
    mutationKey: ["buyer-room-thread-reply"],
    mutationFn: async (input: { threadId: string; body: string }) => {
      const { data, error, response } = await api.POST(
        "/public/rooms/threads/{threadId}/comments",
        {
          params: { path: { threadId: input.threadId } },
          body: { body: input.body },
          ...bearer(token),
        },
      );
      if (error) {
        refuseOrThrow(error, response, t);
      }
      return data;
    },
    onError: retireOnRefusal(onSessionLost),
    onSuccess: refresh,
  });
  const documents: BoardDocument[] = (docs.data?.data ?? []).map((doc) => ({
    id: doc.id,
    groupKey: doc.group_key,
    title: doc.title,
    meta: doc.filename,
    actions: <BuyerDocumentVerbs token={token} doc={doc} />,
  }));
  return (
    <QueryStates
      query={docs}
      pendingLines={3}
      pendingLabel={t("buyer.docs.title")}
    >
      <QueryStates
        query={threads}
        pendingLines={3}
        pendingLabel={t("buyer.docs.title")}
      >
        {docs.data && threads.data ? (
          <DocumentBoard
            title={t("buyer.docs.title")}
            sub={t("buyer.docs.sub")}
            groups={DOCUMENT_GROUPS.map((g) => ({
              key: g.key,
              label: t(g.labelKey),
            }))}
            documents={documents}
            threads={threads.data.data}
            empty={t("buyer.docs.empty")}
            verbs={{
              mayRequireChange: true,
              refusal,
              open: mayWrite ? (input) => open.mutateAsync(input) : undefined,
              reply: mayWrite
                ? (threadId, body) => reply.mutateAsync({ threadId, body })
                : undefined,
            }}
          />
        ) : null}
      </QueryStates>
    </QueryStates>
  );
}

const ACCESS_TITLE: Record<string, MessageKey> = {
  paused: "buyer.pausedTitle",
  expired: "buyer.expiredTitle",
};

// Why this reader may not write in the conversation, in the order that
// binds first: a preview never writes, a room that is not open takes nothing
// more, a read-only seat may only read. Undefined when they may.
//
// The access test names the ONE state that admits a write rather than the
// states that refuse one. `BuyerRoomAccess` is a plain string on the wire, not
// a union, so the compiler cannot say which values exist and a fifth state
// added on the server would reach this untouched — listing the refusals means
// it arrives here writable, which is the wrong way for a write gate to be
// wrong. `paused` and `expired` do not reach this code today (RoomView answers
// them with their own screen first), and this does not rely on that.
function conversationRefusal(
  view: BuyerRoomView,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (view.preview) {
    return t("buyer.previewReadOnly");
  }
  if (view.access === "closed") {
    return t("buyer.closed");
  }
  if (view.access !== "live") {
    // Any other non-live state, including one this build has never heard of.
    // `buyer.closedNote` rather than `buyer.closed`: "this room is closed" is
    // a claim, and it is false for a paused room.
    return t("buyer.closedNote");
  }
  if (view.participant.capability === "view") {
    return t("threads.readOnly");
  }
  return undefined;
}

function RoomView({
  view,
  token,
  onSessionLost,
}: Readonly<{
  view: BuyerRoomView;
  token: string;
  onSessionLost: () => void;
}>) {
  const t = useT();
  const steward = view.steward_name ?? t("buyer.stewardUnknown");
  // Whether this reader may write is the ANSWER to `conversationRefusal`, not
  // a second opinion sitting beside it: a reader who was given a reason may
  // not write, and one who may write has no reason to show. Spelled apart the
  // two drifted — the refusal named a preview and the write test never
  // mentioned one, so a preview whose seat carried `comment` would have been
  // handed a live composer. That combination does not arise today only
  // because the server mints every preview seat read-only.
  const writeRefusal = conversationRefusal(view, t);
  if (view.access === "paused" || view.access === "expired") {
    return (
      <>
        {view.preview ? (
          <Callout tone="info">{t("buyer.previewBanner")}</Callout>
        ) : null}
        <Panel title={t(ACCESS_TITLE[view.access])}>
          <PanelBody>
            <p>
              {t(
                view.access === "paused"
                  ? "buyer.pausedBody"
                  : "buyer.expiredBody",
                { steward },
              )}
            </p>
          </PanelBody>
          {view.access === "expired" ? (
            <PanelBody>
              <LinkRequest />
            </PanelBody>
          ) : null}
        </Panel>
      </>
    );
  }
  if (!view.room) {
    // A seat that resolves always has a room to read: the server withholds
    // content only in the paused and expired states, both handled above. This
    // is the impossible branch, and it says so rather than rendering a blank.
    return (
      <Panel title={t("buyer.deadTitle")}>
        <PanelBody>
          <p>{t("buyer.deadAskContact")}</p>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <>
      {view.preview ? (
        <Callout tone="info">{t("buyer.previewBanner")}</Callout>
      ) : null}
      <header className="buyer-header">
        <Eyebrow as="span">{t("buyer.eyebrow")}</Eyebrow>
        <h1>{view.room.title}</h1>
        {view.room.welcome_message ? <p>{view.room.welcome_message}</p> : null}
        <p className="t-small buyer-meta">
          {t("buyer.contact", { steward })}
          {view.access === "closed" ? ` ${t("buyer.closedNote")}` : ""}
        </p>
      </header>
      <BuyerBoard
        token={token}
        onSessionLost={onSessionLost}
        mayWrite={writeRefusal === undefined}
        refusal={writeRefusal}
      />
    </>
  );
}
