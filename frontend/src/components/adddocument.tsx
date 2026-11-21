// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Button,
  Field,
  Modal,
  SegmentedControl,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ChoiceList } from "../design-system/choicelist";
import { FileDropzone } from "../design-system/filedropzone";
import { Heading } from "../design-system/heading";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { Select } from "../design-system/select";
import { searchWords } from "../format/collate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { fileListKeys } from "../lib/attachmentkeys";
import {
  ACCEPTED_ATTACHMENT_ATTR,
  type AttachmentParent,
  uploadAttachment,
} from "../lib/attachmentupload";
import { useCanWrite } from "../lib/capability";
import { useHealthScope } from "../lib/healthscope";
import { labelFor } from "../lib/labelfor";
import { problemMessageOf, unwrap } from "../lib/problem";
import { formatUploadLimit, useMaxUploadBytes } from "../lib/uploadlimit";
import {
  CATEGORY_KEYS,
  type Choosable,
  SOURCE_WORDS,
  SOURCES,
  type Source,
  UPLOADABLE_CATEGORIES,
} from "./adddocument.choices";
import {
  DEAL_MATCH_LIMIT,
  DEAL_PAGE_FRESH_MS,
  DEAL_PAGE_SIZE,
  DEAL_SEARCH_REACH,
  walkAccountDeals,
} from "./adddocument.dealsearch";
import {
  addLink,
  type LinkSubmission,
  linkRefusal,
  useLinkTitle,
} from "./adddocument.link";
import { LinkAddressField } from "./adddocument.linkfield";
import { uploadRefusal } from "./adddocument.upload";

// Adding a document to the record the dialog was opened from — an account's
// document library, or a contact's.
//
// Why the parent is a question and not a default: a document on the company is
// about the company; one on a deal is evidence in that deal, and the only kind the
// extraction panel offers to read for deal fields, since a deal is the only record
// the accept can write to.
//
// Why it takes two requests: the upload carries the bytes and the parent; category
// and title go through `PATCH /attachments/{id}/metadata`. That call can fail with
// the file already stored, and the dialog says so rather than reporting a failure
// the reader would answer by uploading the same file twice.
//
// Why the question is only asked on an account: deals hang off a company, so an
// account can offer its own deals as filing targets. A contact sits on deals at
// companies they may not work for, so its library files against the contact.

type DealPage = components["schemas"]["DealListResponse"];

/**
 * The record whose document library this dialog was opened from.
 *
 * `record` is also the wire's own `entity_type`, which is why it is one string
 * rather than two shapes: the upload takes the parent as an (entity_type,
 * entity_id) pair, and the RBAC object the server checks IS that same string
 * (`auth.Require(ctx, entityType, update)`), so a second vocabulary here would
 * be a mapping table with nothing to map.
 */
export type DocumentAnchor = Readonly<{
  record: AttachmentParent["entityType"];
  id: string;
}>;

// Which record the file hangs off: the one this dialog was opened from, or one
// of that account's deals. Two named answers, not a Select carrying a sentinel
// value beside a list of deal ids — filing against the account is a different
// KIND of decision from picking one deal of hundreds.
type Filing = "anchor" | "deal";

/**
 * The parent the bytes are filed against, or null when the reader has chosen
 * "a deal" not yet picked one — a refusal to state, not a parent to guess.
 */
function parentOf(
  anchor: DocumentAnchor,
  filing: Filing,
  deal: RecordPickerCandidate | null,
): AttachmentParent | null {
  if (filing === "deal") {
    return deal ? { entityType: "deal", entityId: deal.id } : null;
  }
  return { entityType: anchor.record, entityId: anchor.id };
}

type Submission = {
  parent: AttachmentParent;
  category: Choosable;
  title: string;
  file: File;
};

// Only what the reader actually chose is sent. A PATCH that also wrote the
// defaults back would overwrite a category the server may have derived for
// itself, and would put this dialog's assumptions into a record it never read.
function metadataFor(submitted: Submission) {
  const title = submitted.title.trim();
  const patch: { category?: Choosable; title?: string } = {};
  if (submitted.category !== "other") {
    patch.category = submitted.category;
  }
  if (title !== "") {
    patch.title = title;
  }
  return patch;
}

export function AddDocumentDialog({
  anchor,
  open,
  onClose,
}: Readonly<{
  anchor: DocumentAnchor;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const titleId = useId();
  const queryClient = useQueryClient();

  const [filing, setFiling] = useState<Filing>("anchor");
  const [deal, setDeal] = useState<RecordPickerCandidate | null>(null);
  const [category, setCategory] = useState<Choosable>("other");
  const [source, setSource] = useState<Source>("file");
  const [title, setTitle] = useState("");
  // Until the reader types a title, a link shows the one its page gives.
  const [titleTyped, setTitleTyped] = useState(false);
  const [url, setUrl] = useState("");
  const [file, setFile] = useState<File | undefined>();
  // Set when the bytes landed and the metadata did not. It is not an error
  // state: the upload succeeded, and the row exists.
  const [partial, setPartial] = useState(false);

  // One number per opening, null while shut. React Query finishes a mutation
  // whoever left, so a result from an earlier opening leaves this draft alone.
  //
  // Written in an EFFECT, not during render: React may discard a render, and a
  // ref assigned in one publishes a value that was never committed.
  const openings = useRef(0);
  const session = useRef<number | null>(null);
  useEffect(() => {
    openings.current += open ? 1 : 0;
    session.current = open ? openings.current : null;
  }, [open]);
  const pressedIn = () => session.current;
  const inSession = (pressed: number | null | undefined) =>
    pressed != null && pressed === session.current;

  // What this installation accepts, so an oversize file is refused here rather
  // than after every byte of it has crossed the wire.
  const maxUploadBytes = useMaxUploadBytes();
  const limitLabel = maxUploadBytes
    ? formatUploadLimit(maxUploadBytes, locale)
    : "";

  // One page of the account's deals, read through the query cache so a second
  // search re-uses what the first walked. Keyed by the CURSOR, because that is
  // what identifies a page of a keyset walk.
  //
  // Built on every anchor, called only from the deal picker — which only the
  // account branch below renders. A hook cannot be conditional, and an
  // uncalled callback asks nothing.
  const readerScope = useHealthScope();
  const fetchDealPage = useCallback(
    (cursor: string | null) =>
      queryClient.fetchQuery({
        queryKey: ["dealsForCompany", anchor.id, readerScope ?? "", cursor],
        staleTime: DEAL_PAGE_FRESH_MS,
        queryFn: async (): Promise<DealPage> => {
          return unwrap(
            await api.GET("/deals", {
              params: {
                query: {
                  company_id: anchor.id,
                  limit: DEAL_PAGE_SIZE,
                  ...(cursor ? { cursor } : {}),
                },
              },
            }),
          );
        },
      }),
    [anchor.id, queryClient, readerScope],
  );

  // Kept on `fetchDealPage` alone. RecordPicker reads a new `searchTargets`
  // identity as a new search space and empties the candidates it is showing, so
  // a callback rebuilt on anything that changes while the reader types would
  // clear the list under them.
  const searchDeals = useCallback(
    (query: string) => walkAccountDeals(fetchDealPage, searchWords(query)),
    [fetchDealPage],
  );

  const parent = parentOf(anchor, filing, deal);
  // The upload's RBAC object IS the parent's entity type, so the grant this
  // dialog checks follows the CHOICE rather than the record it was opened from —
  // asked as the parent names it rather than through a table of the kinds this
  // dialog happens to file against today. The table was that, and it fell
  // behind the endpoint's own list the moment a kind was added to it: a map has
  // to be maintained, and this cannot be wrong.
  //
  // One hook whose ARGUMENT varies, never a hook that appears and disappears:
  // the number of hooks a render performs must not depend on which record the
  // reader is filing against. With "a deal" chosen and none picked yet there is
  // no parent, and the grant that governs the press is still the deal one.
  const permitted = useCanWrite(parent?.entityType ?? "deal", "update");

  // Emptying the form is separate from closing it, because the two happen at
  // different moments: every close empties, and the partial-failure path
  // empties the file without closing.
  const clearDraft = () => {
    setFiling("anchor");
    setDeal(null);
    setCategory("other");
    setSource("file");
    setTitle("");
    setTitleTyped(false);
    setUrl("");
    setFile(undefined);
  };

  // Everything the request needs arrives as a variable. A mutationFn closing
  // over `file` or `parent` would submit whatever the previous render held.
  const upload = useMutation({
    mutationFn: async (submitted: Submission) => {
      const stored = await uploadAttachment(submitted.parent, submitted.file);
      // The stored row, or undefined when the bytes landed and the body could
      // not be read: a document whose id we never learned has nowhere to send
      // its metadata, which the partial-success arm below reports as such.
      const id = stored?.id;
      // The stored row names the company the server filed it under.
      const home = { ...submitted.parent, companyId: stored?.company_id };
      const patch = metadataFor(submitted);
      if (Object.keys(patch).length === 0) {
        return { filed: true, home };
      }
      if (id === undefined) {
        // Stored, but we never learned which row — so there is nothing to
        // address the metadata request to. Partial, for the same reason a
        // refused PATCH is partial: the document exists either way.
        return { filed: false, home };
      }
      // EVERY way the second call can fail is a partial success, not a
      // failure — a refusal, a dropped connection, a parse error alike. Once
      // the bytes are stored, the only wrong answer is the one that tells the
      // reader nothing was. Catching is what covers the thrown half: an
      // openapi-fetch rejection never reaches `error`, and left uncaught it
      // would surface as "Nothing was stored" over a document that is.
      try {
        const { error } = await api.PATCH("/attachments/{id}/metadata", {
          params: { path: { id } },
          body: patch,
        });
        return { filed: !error, home };
      } catch {
        return { filed: false, home };
      }
    },
    onMutate: pressedIn,
    onSuccess: async (result, _submitted, pressed) => {
      const here = inSession(pressed);
      // Closed before the refetch, so a dialog reopened meanwhile keeps its draft.
      if (result.filed && here) {
        closeAndClear();
      }
      for (const queryKey of fileListKeys(result.home)) {
        await queryClient.invalidateQueries({ queryKey });
      }
      if (result.filed || !here) {
        return;
      }
      // Kept open, with the draft cleared: the document is on the record, so
      // offering the same upload again would file it twice.
      clearDraft();
      setPartial(true);
    },
  });

  const link = useMutation({
    mutationFn: async (submitted: LinkSubmission) => {
      const row = await addLink(submitted);
      return { ...submitted.parent, companyId: row.company_id };
    },
    onMutate: pressedIn,
    onSuccess: async (home, _submitted, pressed) => {
      if (inSession(pressed)) {
        closeAndClear();
      }
      for (const queryKey of fileListKeys(home)) {
        await queryClient.invalidateQueries({ queryKey });
      }
    },
  });

  const preview = useLinkTitle(source === "link", url, title, titleTyped);
  const shownTitle = preview.title;

  // Closing empties the form. The dialog stays mounted while shut, so what is
  // left here starts the next opening, and a later upload would resend a file.
  function closeAndClear() {
    clearDraft();
    setPartial(false);
    upload.reset();
    link.reset();
    onClose();
  }

  const refusal =
    source === "link"
      ? linkRefusal({ url, parent, permitted })
      : uploadRefusal({ file, parent, permitted, maxBytes: maxUploadBytes });

  function submit() {
    if (parent && source === "link") {
      link.mutate({ parent, category, title: shownTitle, url });
    } else if (parent && file) {
      upload.mutate({ parent, category, title, file });
    }
  }

  return (
    <Modal
      open={open}
      onClose={closeAndClear}
      labelledBy={titleId}
      intent="form"
    >
      <Heading size="large" id={titleId} className="modal-title">
        {t("docs.add.title")}
      </Heading>
      <div className="form-stack">
        <SegmentedControl
          label={t("docs.add.source")}
          options={SOURCES}
          value={source}
          onChange={setSource}
          labels={{
            file: t("docs.add.sourceFile"),
            link: t("docs.add.sourceLink"),
          }}
        />
        {partial && (
          <Callout
            tone="warning"
            kind="outcome"
            title={t("docs.add.partialTitle")}
          >
            {t("docs.add.partial")}
          </Callout>
        )}
        {upload.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("docs.add.failedTitle")}
          >
            {/* Oversize and permission denied need different next moves. */}
            {problemMessageOf(upload.error, t, t("docs.add.failed"))}
          </Callout>
        )}
        {link.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("docs.add.linkFailedTitle")}
          >
            {problemMessageOf(link.error, t, t("docs.add.linkFailed"))}
          </Callout>
        )}
        {source === "link" && (
          <LinkAddressField url={url} onChange={setUrl} hint={preview.hint} />
        )}
        {/* Not a dropdown: a menu hides the alternative this choice is about.
            Only an account asks: a contact's library has no second answer. */}
        {anchor.record === "company" && (
          <>
            <ChoiceList
              legend={t("docs.add.about")}
              value={filing}
              onChange={setFiling}
              choices={[
                { value: "anchor", label: t("docs.add.thisCompany") },
                {
                  value: "deal",
                  label: t("docs.add.aDeal"),
                  description: t("docs.add.aboutHint"),
                },
              ]}
            />
            {filing === "deal" && (
              <div className="field">
                <RecordPicker
                  label={t("docs.add.dealSearch")}
                  searchTargets={searchDeals}
                  selected={deal}
                  onPick={setDeal}
                  disabled={upload.isPending || link.isPending}
                />
                {/* Always shown: RecordPicker cannot say which search ran out. */}
                <p className="t-caption">
                  {t("docs.add.dealSearchReach", {
                    deals: formatNumber(DEAL_SEARCH_REACH, locale),
                    matches: formatNumber(DEAL_MATCH_LIMIT, locale),
                  })}
                </p>
              </div>
            )}
          </>
        )}
        <Field label={t("docs.add.category")}>
          {(control) => (
            <Select
              {...control}
              value={category}
              onChange={(picked) => setCategory(picked as Choosable)}
              options={UPLOADABLE_CATEGORIES.map((key) => ({
                value: key,
                label: t(labelFor(CATEGORY_KEYS, key)),
              }))}
            />
          )}
        </Field>
        <Field
          label={t("docs.add.name")}
          hint={t(SOURCE_WORDS[source].nameHint)}
        >
          {(control) => (
            <TextInput
              {...control}
              value={shownTitle}
              onChange={(event) => {
                setTitle(event.target.value);
                setTitleTyped(true);
              }}
            />
          )}
        </Field>
        {/* The limit is set per deployment, so the hint waits for the server. */}
        {source === "file" && (
          <FileDropzone
            label={t("docs.add.file")}
            hint={
              limitLabel
                ? t("docs.add.fileHint", { size: limitLabel })
                : undefined
            }
            emptyLabel={t("docs.add.fileEmpty")}
            file={file}
            onPick={setFile}
            accept={ACCEPTED_ATTACHMENT_ATTR}
          />
        )}
      </div>
      <div className="actions">
        <Button onClick={closeAndClear}>{t("docs.add.cancel")}</Button>
        <Button
          variant="primary"
          reason={refusal ? t(refusal, { size: limitLabel }) : undefined}
          pending={upload.isPending || link.isPending}
          busyLabel={t(SOURCE_WORDS[source].busy)}
          onClick={submit}
        >
          {t(SOURCE_WORDS[source].submit)}
        </Button>
      </div>
    </Modal>
  );
}
