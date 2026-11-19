// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useSettledValue } from "../../design-system/debouncedsearch";
import type { MessageKey } from "../../i18n/en";
import type { AttachmentParent } from "../../lib/attachmentupload";
import { unwrap } from "../../lib/problem";
import type { Choosable } from "./adddocument.choices";

type LinkPreview = components["schemas"]["AttachmentLinkPreview"];

const WEB_ADDRESS = /^\s*https?:\/\/[^\s/]+/i;

// A page names itself rarely and slowly; one read per address is enough.
const PREVIEW_FRESH_MS = 10 * 60 * 1000;

/** The address as typed looks like one the server could list. */
export function looksLikeWebAddress(typed: string): boolean {
  return WEB_ADDRESS.test(typed);
}

/**
 * What the page at `typed` calls itself, once the reader has stopped typing.
 * `hint` says why no title came, so the reader knows to type one.
 */
export function useLinkPreview(typed: string): Readonly<{
  title: string | undefined;
  hint: MessageKey | undefined;
}> {
  const settled = useSettledValue(typed.trim());
  const ready = looksLikeWebAddress(settled);
  const preview = useQuery({
    queryKey: ["attachmentLinkPreview", settled],
    enabled: ready,
    staleTime: PREVIEW_FRESH_MS,
    retry: false,
    queryFn: async (): Promise<LinkPreview> =>
      unwrap(
        await api.POST("/attachments/links/preview", {
          body: { url: settled },
        }),
      ),
  });
  if (!ready) {
    return { title: undefined, hint: undefined };
  }
  if (preview.isPending) {
    return { title: undefined, hint: "docs.add.linkReading" };
  }
  const read = preview.data;
  if (read?.title) {
    return { title: read.title, hint: "docs.add.linkTitled" };
  }
  // A Google file answers a sign-in page, so the server does not ask.
  if (read && !read.read && /google\.com$/i.test(read.host)) {
    return { title: undefined, hint: "docs.add.linkGoogle" };
  }
  return { title: undefined, hint: "docs.add.linkUnread" };
}

/**
 * The title a link is added with: the one the user typed, or until they type,
 * the one its page gives. `hint` says where it came from.
 */
export function useLinkTitle(
  active: boolean,
  url: string,
  typed: string,
  touched: boolean,
): Readonly<{ title: string; hint: MessageKey | undefined }> {
  const preview = useLinkPreview(active ? url : "");
  const fromPage = active && !touched ? preview.title : undefined;
  return { title: fromPage ?? typed, hint: preview.hint };
}

export type LinkSubmission = Readonly<{
  parent: AttachmentParent;
  url: string;
  title: string;
  category: Choosable;
}>;

/** Lists the link on its parent and answers the row the server wrote. */
export async function addLink(submitted: LinkSubmission) {
  const title = submitted.title.trim();
  return unwrap(
    await api.POST("/attachments/links", {
      body: {
        entity_type: submitted.parent.entityType,
        entity_id: submitted.parent.entityId,
        url: submitted.url.trim(),
        ...(title === "" ? {} : { title }),
        ...(submitted.category === "other"
          ? {}
          : { category: submitted.category }),
      },
    }),
  );
}

// Why the link cannot be added yet, in the order the reader can act on.
export function linkRefusal({
  url,
  parent,
  permitted,
}: Readonly<{
  url: string;
  parent: AttachmentParent | null;
  permitted: boolean;
}>): MessageKey | null {
  if (!permitted) {
    return "docs.add.errRefused";
  }
  if (!looksLikeWebAddress(url)) {
    return "docs.add.errNoUrl";
  }
  if (!parent) {
    return "docs.add.errNoDeal";
  }
  return null;
}
