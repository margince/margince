// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ExternalLink } from "lucide-react";
import { type ReactNode, useId } from "react";
import { AppLink } from "../design-system/applink";
import { CopyAction } from "../design-system/clipboardcopy";
import { type Fact, FactList } from "../design-system/factlist";
import { Heading } from "../design-system/heading";
import { Logomark } from "../design-system/logomark";
import { Modal } from "../design-system/modal";
import { OffsiteLink } from "../design-system/offsitelink";
import { type Translator, useT } from "../i18n";
import {
  type BuildRelease,
  RELEASE_NOTES_HREF,
  REPOSITORY,
} from "../lib/buildrelease";
import { comparableRelease, shortRevision } from "../lib/release";
import "./aboutmargince.css";

const WEBSITE_HREF = "https://margince.com/";
const LICENSING_HREF = `${REPOSITORY}/blob/main/LICENSING.md`;
const THIRD_PARTY_LICENSES_HREF = "/licenses/THIRD-PARTY-NOTICES.txt";

function knownRevision(build: BuildRelease): string | undefined {
  return comparableRelease(build.revision) ? build.revision : undefined;
}

/**
 * The build as one line of plain text, for a problem report. English in every
 * locale: it is pasted into the core's issue tracker.
 */
export function buildReport(build: BuildRelease): string {
  const revision = knownRevision(build);
  const commit = revision ? ` (commit ${revision})` : "";
  switch (build.kind) {
    case "instance":
      return build.instance
        ? `Margince core ${build.core}, instance ${build.instance}${commit}`
        : `Margince core ${build.core}${commit}`;
    case "core":
      return `Margince ${build.release}${commit}`;
    default:
      return `Margince development build${commit}`;
  }
}

function NewTabLink({
  href,
  children,
}: Readonly<{ href: string; children: ReactNode }>) {
  const t = useT();
  const content = (
    <>
      {children}
      <span className="sr-only"> {t("common.opensNewTab")}</span>
      <ExternalLink aria-hidden />
    </>
  );
  if (href.startsWith("/")) {
    return (
      <AppLink
        className="link-button"
        href={href}
        target="_blank"
        rel="noopener noreferrer"
      >
        {content}
      </AppLink>
    );
  }
  return <OffsiteLink href={href}>{content}</OffsiteLink>;
}

// An instance build names both repositories, even when the two agree.
function versionFacts(build: BuildRelease, t: Translator): Fact[] {
  switch (build.kind) {
    case "instance":
      return [
        ...(build.instance
          ? [
              {
                key: "instance",
                term: t("about.instance"),
                value: build.instance,
              },
            ]
          : []),
        { key: "core", term: t("about.core"), value: build.core },
      ];
    case "core":
      return [
        { key: "version", term: t("about.version"), value: build.release },
      ];
    default:
      return [
        {
          key: "version",
          term: t("about.version"),
          value: t("about.devBuild"),
        },
      ];
  }
}

// Only a core build's commit is in the core repository; an instance's is in
// the repository of whoever built it.
function commitFacts(build: BuildRelease, t: Translator): Fact[] {
  const revision = knownRevision(build);
  const short = shortRevision(revision);
  if (!revision || !short) {
    return [];
  }
  const value =
    build.kind === "core" ? (
      <NewTabLink href={`${REPOSITORY}/commit/${revision}`}>{short}</NewTabLink>
    ) : (
      short
    );
  return [{ key: "commit", term: t("about.commit"), value }];
}

/** What this build of Margince is, under which license, and where it lives. */
export function AboutMargince({ build }: Readonly<{ build: BuildRelease }>) {
  const t = useT();
  return (
    <div className="aboutmargince">
      <div className="aboutmargince-brand">
        <Logomark size={28} />
        {t("shell.logoAria")}
      </div>
      <div className="aboutmargince-build">
        <FactList
          flat
          facts={[...versionFacts(build, t), ...commitFacts(build, t)]}
        />
        <CopyAction
          text={buildReport(build)}
          labels={{
            copy: t("about.copy"),
            copied: t("about.copied"),
            remedy: t("about.copyRemedy"),
          }}
        />
      </div>
      <FactList
        flat
        facts={[
          {
            key: "license",
            term: t("about.license"),
            value: (
              <NewTabLink href={LICENSING_HREF}>
                {t("about.licenseName")}
              </NewTabLink>
            ),
            note: t("about.licenseConversion"),
          },
        ]}
      />
      <p className="t-caption">{t("about.copyright")}</p>
      <ul className="aboutmargince-links">
        <li>
          <NewTabLink href={WEBSITE_HREF}>{t("about.website")}</NewTabLink>
        </li>
        <li>
          <NewTabLink href={REPOSITORY}>{t("about.source")}</NewTabLink>
        </li>
        <li>
          <NewTabLink href={RELEASE_NOTES_HREF}>
            {t("shell.help.releaseNotes")}
          </NewTabLink>
        </li>
        <li>
          <NewTabLink href={THIRD_PARTY_LICENSES_HREF}>
            {t("about.thirdParty")}
          </NewTabLink>
        </li>
      </ul>
    </div>
  );
}

export function AboutMarginceDialog({
  build,
  open,
  onClose,
  returnFocusTo,
}: Readonly<{
  build: BuildRelease;
  open: boolean;
  onClose: () => void;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  const titleId = useId();
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      intent="form"
      returnFocusTo={returnFocusTo}
      // Not the first focusable: on a core build that is the commit link, and
      // an Enter on arrival would leave the app.
      initialFocusTo={() =>
        document
          .getElementById(titleId)
          ?.closest<HTMLElement>("[role=dialog]")
          ?.querySelector<HTMLElement>(
            ".aboutmargince-build .copy-action button",
          ) ?? null
      }
    >
      <Heading size="large" id={titleId} className="modal-title">
        {t("about.title")}
      </Heading>
      <AboutMargince build={build} />
    </Modal>
  );
}
