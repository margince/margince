import {
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ArrowRight, RefreshCw, ShieldCheck } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { usePublishSelection } from "../app/attention";
import { useUnsavedGuard } from "../app/unsaved";
import {
  Badge,
  Button,
  Checkbox,
  Field,
  Modal,
  Radio,
  SectionHeader,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../design-system/drawerbands";
import { Eyebrow } from "../design-system/eyebrow";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody, PanelIntro, PanelRow } from "../design-system/panel";
import { WriteRefused } from "../design-system/querystates";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { FieldDiff } from "../design-system/trust";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCanUpsert, useHoldsAdminRole } from "../lib/capability";
import { problemMessageOf, throwProblem, unwrap } from "../lib/problem";
import { navigate } from "../lib/router";
import { useMe } from "../lib/session";
import { coldFieldLabel } from "./coldfieldlabels";
import { ReadWarnings } from "./company-context.notices";
import { CompanyFactsCard, PROFILE_GROUPS } from "./companycontextfacts";
import { CompanyMark } from "./companymark";
import { storeCompany, useCompany } from "./installationcompany";
import "./company-context.css";

type Capabilities = components["schemas"]["CompanyContextCapabilities"];
type CompanyProfile = components["schemas"]["CompanyProfile"];
type CompanyInput = components["schemas"]["CompanyProfileInput"];
type SiteRead = components["schemas"]["CompanySiteRead"];
type Comparison = components["schemas"]["CompanySiteReadComparison"];
type Resolution = components["schemas"]["CompanySiteReadResolution"];

/** What the reviewer had on screen at the moment they applied the refresh. */
type RefreshChoice = Readonly<{
  current: CompanyInput;
  read: SiteRead;
  selected: Set<string>;
  resolutions: Record<string, Resolution>;
}>;

const EMPTY_COMPANY_INPUT: CompanyInput = {
  display_name: "",
  website: "",
  offer_summary: "",
  icp: "",
};

// Joins comparison keys into the one string the default selection is keyed on.
// A NUL can appear in no key the server mints, so no key can ever split into
// two — which a separator drawn from ordinary punctuation could not promise.
const SELECTION_SEPARATOR = "\u0000";

const MULTILINE_FIELDS = new Set<keyof CompanyInput>([
  "offer_summary",
  "icp",
  "value_proposition",
  "customer_pains",
  "desired_outcomes",
  "buying_center",
  "buying_intents",
  "common_objections",
  "sales_motion",
  "history",
]);

// The rollout answer every surface that gates on the flag shares — the card
// here and the onboarding journey. Named so a caller can ask
// the cache whether the answer has LANDED, which is a different question from
// whether the request went out.
export const companyContextCapabilitiesQueryKey = [
  "company-context-capabilities",
];

export function useCompanyContextCapabilities(enabled = true) {
  return useQuery({
    queryKey: companyContextCapabilitiesQueryKey,
    enabled,
    queryFn: async (): Promise<Capabilities> => {
      return unwrap(await api.GET("/company/context/capabilities"));
    },
  });
}

// ManualCompanySetup is the rollback-safe first-run floor below the
// `onboarding` rollout stage. It creates the same canonical profile with the
// same semantic minimum, without exposing the new five-step experience.
export function ManualCompanySetup() {
  const t = useT();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<CompanyInput>(EMPTY_COMPANY_INPUT);
  const save = useMutation({
    mutationFn: async () => {
      return unwrap(
        await api.PUT("/company", {
          body: trimCompanyInput(form),
        }),
      );
    },
    onSuccess: (profile) => {
      storeCompany(queryClient, profile);
      navigate({ screen: "home" });
    },
  });
  return (
    // One Panel, in the ONE lead tone, where a gradient with a decorative
    // circle and two bespoke boxes used to be. The heading is the panel's own
    // title rather than a bare h2: preflight leaves an unclassed heading at
    // body size, so the page's lead sentence used to render as body text
    // inside a gradient.
    <div className="wrap narrow">
      <Panel
        tone="accent"
        title={
          <>
            <ShieldCheck aria-hidden size={16} />
            {t("settings.companyManualTitle")}
          </>
        }
        actions={
          <Button
            variant="primary"
            disabled={!requiredComplete(form) || save.isPending}
            onClick={() => save.mutate()}
          >
            {t("settings.companyCreateWorkspace")} <ArrowRight aria-hidden />
          </Button>
        }
      >
        <PanelBody className="form-stack">
          <Eyebrow>{t("settings.companyManualKicker")}</Eyebrow>
          <p>{t("settings.companyManualSub")}</p>
          {(["display_name", "offer_summary", "icp"] as const).map((field) => (
            <Field key={field} label={coldFieldLabel(field, t)}>
              {(control) =>
                field === "display_name" ? (
                  <TextInput
                    {...control}
                    value={String(form[field] ?? "")}
                    onChange={(event) =>
                      setForm({ ...form, [field]: event.target.value })
                    }
                  />
                ) : (
                  <Textarea
                    {...control}
                    rows={4}
                    value={String(form[field] ?? "")}
                    onChange={(event) =>
                      setForm({ ...form, [field]: event.target.value })
                    }
                  />
                )
              }
            </Field>
          ))}
          {save.isError && (
            <WriteRefused
              titleKey="settings.companySaveFailed"
              error={save.error}
            />
          )}
        </PanelBody>
      </Panel>
    </div>
  );
}

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
  if (!isAdmin) {
    return null;
  }

  if (capabilities.data && !capabilities.data.read_enabled) {
    return null;
  }

  // Waits for the /me probe, so a reader who may edit never sees it flash.
  const readOnly = me.isSuccess && !canEdit;

  return (
    <div className="company-context-shell">
      {/* The company's face first: what a reader recognises at a glance. */}
      {company.data && !company.isError && form && (
        <CompanyMark
          profile={company.data}
          canEdit={canEdit}
          readOnly={readOnly}
        />
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
          error={save.isError ? problemMessageOf(save.error, t) : null}
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
          error={confirm.error ? problemMessageOf(confirm.error, t) : undefined}
        />
      )}
    </div>
  );
}

/**
 * WHERE we read it from, and the verb that reads it again.
 *
 * Its own card because it answers a different question from the one above: that
 * card says what we hold, this one says which site we hold it from and offers
 * the reading. The trust rule rides here for the same reason — that website
 * text never becomes instructions is a promise about the READ, and on the plate
 * above it read as a caption on the profile.
 */
function CompanySourceCard({
  form,
  canEdit,
  refreshing,
  failure,
  onEdit,
  onRefresh,
}: Readonly<{
  form: CompanyInput;
  canEdit: boolean;
  refreshing: boolean;
  /** What went wrong with the last read, in words a reader can act on. */
  failure: string | null;
  onEdit: () => void;
  onRefresh: () => void;
}>) {
  const t = useT();
  const website = (form.website ?? "").trim();
  return (
    <Panel title={t("settings.companySourceTitle")}>
      <PanelBody>
        <PanelIntro>{t("settings.companyTrust")}</PanelIntro>
      </PanelBody>
      <SettingList bleed="settings">
        <SettingRow
          label={t("settings.companyWebsite")}
          description={t("settings.companyWebsiteHint")}
          value={website === "" ? t("field.unset") : website}
          control={
            canEdit ? (
              <Button
                variant="ghost"
                aria-label={t("settings.companyEditField", {
                  field: t("settings.companyWebsite"),
                })}
                onClick={onEdit}
              >
                {t("settings.companyEdit")}
              </Button>
            ) : null
          }
        />
        {/* Reading the website rewrites this profile, so the server asks for
            the same grant as a save. */}
        {canEdit && (
          <SettingRow
            label={t("settings.companyRefreshRow")}
            description={t("settings.companyRefreshHint")}
            control={
              <Button
                variant="primary"
                reason={
                  website === ""
                    ? t("settings.companyWebsiteRequired")
                    : undefined
                }
                pending={refreshing}
                onClick={onRefresh}
              >
                <RefreshCw aria-hidden /> {t("settings.companyRefresh")}
              </Button>
            }
          />
        )}
      </SettingList>
      {failure !== null && (
        <PanelBody>
          <div className="settings-panel-commit">
            <WriteRefused
              titleKey="settings.companyRefreshFailed"
              message={failure}
            />
          </div>
        </PanelBody>
      )}
    </Panel>
  );
}

/**
 * The one form every row's Edit verb opens. One drawer, because ONE PUT writes
 * the profile: a dialog per group would commit the others' unsaved drafts.
 */
function CompanyProfileDialog({
  form,
  focus,
  pending,
  error,
  onChange,
  onClose,
  onSubmit,
}: Readonly<{
  form: CompanyInput;
  /** Which row's Edit was pressed, and so where focus lands. */
  focus: keyof CompanyInput;
  pending: boolean;
  error: string | null;
  onChange: (next: CompanyInput) => void;
  onClose: () => void;
  onSubmit: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const formId = useId();
  // Not `autoFocus`: the a11y lint refuses it. A callback, because the field
  // it lands on is an input for some rows and a textarea for others.
  const asked = useRef<HTMLElement | null>(null);
  const capture = (node: HTMLElement | null) => {
    asked.current = node;
  };
  useEffect(() => {
    asked.current?.focus();
  }, []);
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2">
          {t("settings.companyContextTitle")}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        <form
          id={formId}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit();
          }}
        >
          {/* The site the read starts from, above the statements a read would
              propose changes to: it is the precondition for everything below. */}
          <CompanyFieldInput
            field="website"
            form={form}
            asked={focus === "website" ? capture : undefined}
            onChange={onChange}
          />
          {PROFILE_GROUPS.map((group) => (
            <div className="form-stack" key={group.title}>
              <SectionHeader title={t(group.title)} level={3} />
              {group.fields.map((field) => (
                <CompanyFieldInput
                  key={field}
                  field={field}
                  form={form}
                  asked={focus === field ? capture : undefined}
                  onChange={onChange}
                />
              ))}
            </div>
          ))}
          {error !== null && (
            <WriteRefused
              titleKey="settings.companySaveFailed"
              message={error}
            />
          )}
        </form>
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button variant="ghost" type="button" onClick={onClose}>
          {t("create.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={!pending && !requiredComplete(form)}
          pending={pending}
          busyLabel={t("common.saving")}
        >
          {t("settings.companySave")}
        </Button>
      </DrawerFoot>
    </Modal>
  );
}

/** One field of that form; `Field` draws the real `<label for>`. */
function CompanyFieldInput({
  field,
  form,
  asked,
  onChange,
}: Readonly<{
  field: keyof CompanyInput;
  form: CompanyInput;
  /** Set on the ONE field whose Edit opened this dialog, so focus lands there. */
  asked?: (node: HTMLElement | null) => void;
  onChange: (next: CompanyInput) => void;
}>) {
  const t = useT();
  const value = String(form[field] ?? "");
  const label =
    field === "website"
      ? t("settings.companyWebsite")
      : coldFieldLabel(field, t);
  return (
    <Field label={label}>
      {(control) =>
        MULTILINE_FIELDS.has(field) ? (
          <Textarea
            {...control}
            ref={asked}
            rows={3}
            value={value}
            onChange={(event) =>
              onChange({ ...form, [field]: event.target.value })
            }
          />
        ) : (
          <TextInput
            {...control}
            ref={asked}
            value={value}
            onChange={(event) =>
              onChange({ ...form, [field]: event.target.value })
            }
          />
        )
      }
    </Field>
  );
}

function RefreshReview(
  props: Readonly<{
    read: SiteRead;
    selected: Set<string>;
    resolutions: Record<string, Resolution>;
    onToggle: (key: string) => void;
    onResolve: (resolution: Resolution) => void;
    onConfirm: () => void;
    /** The same answer the save asks for: applying a read WRITES the profile.
     *  Carried rather than re-derived so a grant revoked while a reviewer sits
     *  on this screen takes the apply with it — the /me snapshot refreshes on
     *  window focus, and the review outlives that. */
    canApply: boolean;
    confirming: boolean;
    error?: string;
  }>,
) {
  const t = useT();
  const { locale } = useLocale();
  const ready =
    props.read.status === "ready" || props.read.status === "partial";
  const conflicts = props.read.comparisons.filter(
    (item) => item.classification === "human_conflict",
  );
  const unresolved = conflicts.some((item) => {
    const resolution = props.resolutions[item.key];
    return (
      !resolution ||
      (resolution.action === "use_value" && !(resolution.value ?? "").trim())
    );
  });
  const coverage =
    props.read.pages.length === 0
      ? 0
      : Math.round(
          (props.read.pages.filter((page) => page.status === "fetched").length /
            props.read.pages.length) *
            100,
        );
  return (
    // The review as a Panel: the state sentence is its title (it was a bare
    // h3, which preflight draws at body size), the comparisons are full-bleed
    // rows, and the coverage figure sits in the footer band because it belongs
    // to the whole read rather than to any one row. It used to be 24px — larger
    // than the page's own h1 — for a number nobody acts on.
    <Panel
      title={
        ready
          ? t("settings.companyRefreshReady")
          : t("settings.companyRefreshReading")
      }
      footer={
        <span className="company-context-coverage">
          <strong>{formatNumber(coverage, locale)}%</strong>{" "}
          {t("settings.companyCoverage")}
        </span>
      }
      actions={
        <>
          {props.error && (
            <WriteRefused
              titleKey="settings.companyApplyFailed"
              message={props.error}
            />
          )}
          {/* An unresolved conflict refuses the verb, so it is the verb's own
              `reason` rather than a second notice beside it: the sentence and
              the refusal cannot then disagree. */}
          {props.canApply && (
            <Button
              variant="primary"
              disabled={!ready || props.confirming}
              reason={unresolved ? t("settings.companyResolveAll") : undefined}
              onClick={props.onConfirm}
            >
              {t("settings.companyApplyRefresh")} <ArrowRight aria-hidden />
            </Button>
          )}
        </>
      }
    >
      {/* What this panel IS, under the title that says where the read has got
          to. Beside the title it would squeeze a sentence into a column on a
          phone, for the same reason the profile panel's eyebrow sits here. */}
      <PanelBody className="form-stack">
        <Eyebrow>{t("settings.companyRefreshReview")}</Eyebrow>
        <ReadWarnings warnings={props.read.warnings} />
      </PanelBody>
      {props.read.comparisons.map((item) => (
        <ComparisonRow
          key={`${item.value_kind}:${item.key}`}
          item={item}
          selected={props.selected.has(item.key)}
          resolution={props.resolutions[item.key]}
          onToggle={() => props.onToggle(item.key)}
          onResolve={props.onResolve}
        />
      ))}
    </Panel>
  );
}

function ComparisonRow(
  props: Readonly<{
    item: Comparison;
    selected: boolean;
    resolution?: Resolution;
    onToggle: () => void;
    onResolve: (resolution: Resolution) => void;
  }>,
) {
  const t = useT();
  const { item } = props;
  const conflict = item.classification === "human_conflict";
  // The field this card is about, named once: it heads the card AND names the
  // checkbox. A row of boxes that all announce the same words is a list a
  // screen reader cannot tell apart, and picking the wrong change here is what
  // gets written to the record.
  const fieldLabel = coldFieldLabel(item.key.split("/").at(-2) ?? item.key, t);
  const selectable = !conflict && item.classification !== "unchanged";
  return (
    <PanelRow
      className={`company-context-comparison is-${item.classification}`}
    >
      <div className="company-context-comparison-title">
        {/* On a selectable row the field name IS the tick's other half, which
            is what makes the words clickable; the aria-label spells the whole
            instruction and contains those words, so the visible name and the
            announced one agree (WCAG 2.5.3). A row with nothing to choose keeps
            the name as plain text rather than a control that does nothing. */}
        {selectable ? (
          <Checkbox
            checked={props.selected}
            onChange={props.onToggle}
            aria-label={t("settings.companySelectChange", {
              field: fieldLabel,
            })}
            label={<strong>{fieldLabel}</strong>}
          />
        ) : (
          <strong>{fieldLabel}</strong>
        )}
        <Badge>
          {t(`settings.companyClass.${item.classification}` as MessageKey)}
        </Badge>
      </div>
      {/* The design system's own old→new diff. A null current value reads as
          the "created" marker rather than as a blank box claiming we held an
          empty string. */}
      <FieldDiff oldValue={item.current_value} newValue={item.proposed_value} />
      {conflict && (
        <div className="company-context-resolutions">
          {(["keep_current", "accept_proposal"] as const).map((action) => (
            <Radio
              key={action}
              name={`resolution-${item.key}`}
              checked={props.resolution?.action === action}
              onChange={() => props.onResolve({ key: item.key, action })}
              label={t(`settings.companyResolution.${action}` as MessageKey)}
            />
          ))}
          <Radio
            name={`resolution-${item.key}`}
            checked={props.resolution?.action === "use_value"}
            onChange={() =>
              props.onResolve({
                key: item.key,
                action: "use_value",
                value: item.current_value ?? "",
              })
            }
            label={t("settings.companyResolution.use_value")}
          />
          {props.resolution?.action === "use_value" && (
            /* Named, because this field decides what gets written to the company
               record and it had no accessible name at all — no label, no
               aria-label, not even a placeholder. It is revealed by the radio
               above it, so it takes that radio's words plus the field this
               conflict is about: "Keep this value" alone would be one of several
               identical names on a page resolving several conflicts. */
            <TextInput
              aria-label={t("settings.companyResolution.useValueFor", {
                field: fieldLabel,
              })}
              value={props.resolution.value ?? ""}
              onChange={(event) =>
                props.onResolve({
                  key: item.key,
                  action: "use_value",
                  value: event.target.value,
                })
              }
            />
          )}
        </div>
      )}
    </PanelRow>
  );
}

function refreshConfirmation(
  current: CompanyInput,
  read: SiteRead,
  selected: Set<string>,
  resolutions: Record<string, Resolution>,
) {
  const profile = { ...current };
  for (const comparison of read.comparisons) {
    if (comparison.value_kind !== "profile_field") {
      continue;
    }
    if (
      selected.has(comparison.key) &&
      comparison.classification !== "human_conflict"
    ) {
      profile[comparison.key as keyof CompanyInput] = comparison.proposed_value;
    }
  }
  const factKeys = read.facts
    .filter((fact) => selected.has(fact.value_key))
    .map((fact) => fact.value_key);
  return {
    draft_version: read.draft_version,
    proposal_hash: read.proposal_hash,
    profile: trimCompanyInput(profile),
    selected_fact_keys: factKeys,
    resolutions: Object.values(resolutions),
  };
}

function requiredComplete(form: CompanyInput): boolean {
  return [form.display_name, form.offer_summary, form.icp].every(
    (value) => String(value ?? "").trim() !== "",
  );
}

function trimCompanyInput(form: CompanyInput): CompanyInput {
  return Object.fromEntries(
    Object.entries(form).map(([key, value]) => [
      key,
      typeof value === "string" ? value.trim() : value,
    ]),
  ) as CompanyInput;
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
