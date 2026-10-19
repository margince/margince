import {
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { usePublishSelection } from "../../app/attention";
import { useUnsavedGuard } from "../../components/unsaved";
import { useT } from "../../i18n";
import { useCanUpsert, useHoldsAdminRole } from "../../lib/capability";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import {
  companySaveMessage,
  storeCompany,
  useCompany,
} from "../sign-in/installationcompany";
import { CompanyFactsCard } from "./companycontextfacts";
import {
  companyContextShown,
  useCompanyContextCapabilities,
} from "./companycontextreach";
import { CompanyMark } from "./companymark";
import "./company-context.css";
import {
  CompanyProfileDialog,
  trimCompanyInput,
} from "./company-context.profile";
import { RefreshReview, refreshConfirmation } from "./company-context.refresh";
import { CompanySourceCard } from "./company-context.source";

export { ManualCompanySetup } from "./company-context.profile";

type CompanyProfile = components["schemas"]["CompanyProfile"];

export type CompanyInput = components["schemas"]["CompanyProfileInput"];

export type SiteRead = components["schemas"]["CompanySiteRead"];

export type Comparison = components["schemas"]["CompanySiteReadComparison"];

export type Resolution = components["schemas"]["CompanySiteReadResolution"];

/** What the reviewer had on screen at the moment they applied the refresh. */
type RefreshChoice = Readonly<{
  current: CompanyInput;
  read: SiteRead;
  selected: Set<string>;
  resolutions: Record<string, Resolution>;
}>;

// Joins comparison keys into the one string the default selection is keyed on.
// A NUL can appear in no key the server mints, so no key can ever split into
// two — which a separator drawn from ordinary punctuation could not promise.
const SELECTION_SEPARATOR = "\u0000";

function profileInput(profile: CompanyProfile): CompanyInput {
  return {
    display_name: profile.display_name,
    website: profileValue(profile, "website"),
    offer_summary: profileValue(profile, "offer_summary"),
    icp: profileValue(profile, "icp"),
    value_proposition: profileValue(profile, "value_proposition"),
    usp: profileValue(profile, "usp"),
    customer_pains: profileValue(profile, "customer_pains"),
    desired_outcomes: profileValue(profile, "desired_outcomes"),
    buying_center: profileValue(profile, "buying_center"),
    buying_intents: profileValue(profile, "buying_intents"),
    common_objections: profileValue(profile, "common_objections"),
    sales_motion: profileValue(profile, "sales_motion"),
    legal_name: profileValue(profile, "legal_name"),
    registered_address: profileValue(profile, "registered_address"),
    register_vat: profileValue(profile, "register_vat"),
    industry: profileValue(profile, "industry"),
    history: profileValue(profile, "history"),
  };
}

function profileValue(
  profile: CompanyProfile,
  field: keyof CompanyProfile,
): string {
  const value = profile[field];
  return typeof value === "string" ? value : "";
}

function absoluteWebsite(raw: string): string {
  const value = raw.trim();
  return /^https?:\/\//i.test(value) ? value : `https://${value}`;
}

// What the refresh area may say when the website read goes wrong. Only the
// START failure speaks verbatim: that problem answers the URL the reader just
// typed — the site is unreachable, robots refused it, the budget is spent —
// and it is guidance they can act on. A status poll that fails answers a read
// id nobody typed, so its detail is machinery talk and the catalog sentence is
// the honest thing to show; the failure itself still reaches the console
// through the shared query cache, which reports every query failure.
function refreshProblem(
  start: Readonly<{ isError: boolean; error: unknown }>,
  poll: Readonly<{ isError: boolean; error: unknown }>,
  t: ReturnType<typeof useT>,
): string | null {
  if (start.isError) {
    return problemMessageOf(start.error, t);
  }
  return poll.isError ? t("settings.companyRefreshUnreadable") : null;
}

export function CompanyContextCard() {
  const t = useT();
  const queryClient = useQueryClient();
  const capabilities = useCompanyContextCapabilities();
  // Writing it is an upsert: the company is one standing record
  // that the first save MINTS, so the server demands `create` when no anchor
  // exists and `update` when one does, deciding inside its own transaction. A
  // client asking for either verb alone would hide the editor from a principal
  // the server would have admitted.
  const me = useMe();
  const canEdit = useCanUpsert("company");
  // The installation's own profile is administered, not read on a grant:
  // useCompany asks only from an admin seat, and for any other this card draws
  // nothing (below). The settings page it sits on opens for readers who are not
  // admins too (settingscatalog.ts, the `company` entry); this card is not theirs.
  const isAdmin = useHoldsAdminRole();
  const company = useCompany(true);
  const [form, setForm] = useState<CompanyInput | null>(null);
  // Which row's Edit was pressed, and so where the dialog puts focus. One
  // dialog holds every field because ONE PUT writes them: a per-group form
  // would promise three independent writes the server does not offer, and the
  // reader would be committing a draft of the other twelve fields unseen.
  const [editing, setEditing] = useState<keyof CompanyInput | null>(null);
  const [readID, setReadID] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  // The agent surface reports what the reader is doing, and a selection is the
  // clearest statement of it a screen can make (app/attention.tsx).
  usePublishSelection(selected.size);
  const [resolutions, setResolutions] = useState<Record<string, Resolution>>(
    {},
  );
  const seeded = useRef<string | null>(null);

  // Seed the editor from the server, and re-seed only when the server SAYS
  // something different — which is not the same question as whether react-query
  // handed over a new object.
  //
  // Every refetch mints one, and this query refetches on window focus like the
  // rest of them: an operator who tabs away to copy their positioning text out
  // of a deck and tabs back triggered a refetch that returned the profile
  // unchanged, and the effect then threw away everything they had typed since
  // the page loaded. Comparing the VALUES leaves an unsaved draft alone across
  // every refetch that changes nothing, and still shows another admin's change
  // when one really lands.
  //
  // This is also the only place the form is seeded. A save or an applied
  // refresh writes the returned profile into the query cache, which arrives
  // here — a second `setForm` at those call sites would be a second writer for
  // one piece of state.
  useEffect(() => {
    if (!company.data) {
      return;
    }
    const next = profileInput(company.data);
    const signature = JSON.stringify(next);
    if (seeded.current === signature) {
      return;
    }
    seeded.current = signature;
    setForm(next);
  }, [company.data]);

  const save = useMutation({
    mutationFn: async (body: CompanyInput) => {
      return unwrap(await api.PUT("/company", { body }));
    },
    onSuccess: (profile) => {
      storeCompany(queryClient, profile);
      // Committing is what the dialog was opened for, so a landed save closes
      // it and leaves the confirmation on the card behind — where the rows the
      // save changed are. A refused save keeps the dialog open with what was
      // typed still in it.
      setEditing(null);
    },
  });

  const startRefresh = useMutation({
    // The website arrives as the mutation's VARIABLE rather than being read off
    // `form` through this closure, and that is a correctness fix rather than a
    // style one. react-query re-arms a mutation's options in a PASSIVE effect,
    // so between the commit that first renders this control with a loaded
    // `form` and the effect that hands the observer that render's closure there
    // is a window where the DOM offers an enabled button and the mutation still
    // holds the previous closure — the one where `form` was null. A click
    // landing in that window read "" and told a reader who has a website to add
    // one. React yields between commit and passive effects, so the window is
    // real in a browser and merely likelier on a loaded machine; it surfaced as
    // a flaky company-context suite failing on the guard below.
    //
    // A variable cannot be older than the button: the handler only exists in a
    // render where `form` is non-null, so what it passes is what the field
    // beside it shows.
    mutationFn: async (candidate: string) => {
      const website = candidate.trim();
      if (!website) {
        throwProblem({ title: t("settings.companyWebsiteRequired") });
      }
      return unwrap(
        await api.POST("/company/site-reads", {
          params: { header: { "Idempotency-Key": crypto.randomUUID() } },
          body: { url: absoluteWebsite(website) },
        }),
      );
    },
    onSuccess: (read) => {
      setReadID(read.id);
      setSelected(new Set());
      setResolutions({});
    },
  });

  const siteRead = useQuery({
    queryKey: ["company-context-refresh", readID],
    queryFn:
      readID !== null
        ? async (): Promise<SiteRead> =>
            unwrap(
              await api.GET("/company/site-reads/{readId}", {
                params: { path: { readId: readID } },
              }),
            )
        : skipToken,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "queued" || status === "reading" ? 900 : false;
    },
  });

  // What the reviewer starts from, as one comparable string. The poll returns
  // a new array every 900ms. The seed effect must run only when new proposals
  // arrive, or it resets the reviewer's ticks.
  const defaultSelection = (siteRead.data?.comparisons ?? [])
    .filter(
      (item) =>
        item.classification === "new" ||
        item.classification === "machine_change",
    )
    .map((item) => item.key)
    .join(SELECTION_SEPARATOR);

  useEffect(() => {
    setSelected(
      new Set(
        defaultSelection === ""
          ? []
          : defaultSelection.split(SELECTION_SEPARATOR),
      ),
    );
  }, [defaultSelection]);

  // Everything the confirm sends arrives as the mutation's VARIABLE, for the
  // reason spelled out on startRefresh above: react-query re-arms a mutation's
  // options in a passive effect, so a click landing between commit and that
  // effect runs the previous render's closure. Read through one, `form` and the
  // read are null and the reviewer is told their refresh is unavailable while
  // it is on the screen in front of them; `selected` and `resolutions` are
  // worse, because a stale pair sends a set of choices nobody made. The click
  // handler belongs to the committed render, so what it passes is what the
  // reviewer sees.
  const confirm = useMutation({
    mutationFn: async (choice: RefreshChoice) => {
      const body = refreshConfirmation(
        choice.current,
        choice.read,
        choice.selected,
        choice.resolutions,
      );
      const { data, error, response } = await api.POST(
        "/company/site-reads/{readId}/confirm",
        {
          params: {
            path: { readId: choice.read.id },
            header: { "Idempotency-Key": crypto.randomUUID() },
          },
          body,
        },
      );
      if (error) {
        if (response.status === 409) {
          throwProblem({ title: t("settings.companyRefreshStale") });
        }
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (profile) => {
      // The editor re-seeds itself from the cache — see the seed effect above,
      // which owns that state — so this only writes what the applied refresh
      // ENDS: the read the reviewer was working through, and the choices they
      // made in it.
      storeCompany(queryClient, profile);
      setReadID(null);
      setResolutions({});
    },
  });

  const refreshFailure = refreshProblem(startRefresh, siteRead, t);
  // Bound to a const so the narrowing below survives into the confirm handler:
  // TypeScript discards a narrowed PROPERTY access inside a closure, and the
  // whole point of that handler is to carry the read rather than re-read it.
  const read = siteRead.data;

  // An unsaved edit, and the window's own question about leaving with one.
  //
  // The draft outlives the dialog on purpose — dismissing the dialog does not
  // destroy what was typed, and reopening Edit shows it again — which means a
  // card whose dialog is closed can still be holding a change nobody has
  // committed. Compared against what the server SAID rather than against the
  // object it said it in, for the same reason the seed effect above does: every
  // refetch mints a new object and none of them is an edit.
  const stored = company.data ? profileInput(company.data) : null;
  const dirty =
    form !== null &&
    stored !== null &&
    JSON.stringify(form) !== JSON.stringify(stored);
  useUnsavedGuard(dirty);

  // Nothing rather than a refusal, for the reason the rollout flag returns
  // nothing: a surface that is none of this reader's work is not a fact being
  // withheld from them, and a card saying so on a tab they are entitled to
  // open would draw a permission boundary as a fault.
  if (!companyContextShown(isAdmin, capabilities.data)) {
    return null;
  }

  // Waits for the /me probe, so a reader who may edit never sees it flash.
  const readOnly = me.isSuccess && !canEdit;

  return (
    <div className="company-context-shell">
      {/* The company's face first: what a reader recognises at a glance. */}
      {company.data && !company.isError && form && (
        <CompanyMark profile={company.data} canEdit={canEdit} />
      )}
      <CompanyFactsCard
        company={company}
        form={form}
        canEdit={canEdit}
        readOnly={readOnly}
        saved={save.isSuccess}
        onEdit={setEditing}
      />
      {form && (
        <CompanySourceCard
          form={form}
          canEdit={canEdit}
          refreshing={startRefresh.isPending}
          failure={refreshFailure}
          onEdit={() => setEditing("website")}
          onRefresh={() => startRefresh.mutate(form.website ?? "")}
        />
      )}
      {editing !== null && form !== null && (
        <CompanyProfileDialog
          form={form}
          focus={editing}
          pending={save.isPending}
          error={save.isError ? companySaveMessage(save.error, t) : null}
          onChange={setForm}
          onClose={() => setEditing(null)}
          onSubmit={() => save.mutate(trimCompanyInput(form))}
        />
      )}
      {read && form && (
        <RefreshReview
          read={read}
          selected={selected}
          resolutions={resolutions}
          onToggle={(key) => setSelected(toggleSet(selected, key))}
          onResolve={(resolution) =>
            setResolutions({
              ...resolutions,
              [resolution.key]: resolution,
            })
          }
          onConfirm={() =>
            confirm.mutate({
              current: form,
              read,
              selected,
              resolutions,
            })
          }
          canApply={canEdit}
          confirming={confirm.isPending}
          error={
            confirm.error ? companySaveMessage(confirm.error, t) : undefined
          }
        />
      )}
    </div>
  );
}

function toggleSet(source: Set<string>, key: string): Set<string> {
  const next = new Set(source);
  if (next.has(key)) {
    next.delete(key);
  } else {
    next.add(key);
  }
  return next;
}
