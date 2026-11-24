import { useEffect, useRef } from "react";
import { Button, Field } from "../../design-system/atoms";
import { TimezoneSelect } from "../../design-system/timezoneselect";
import { useT } from "../../i18n";
import { navigate } from "../../lib/router";

type BookingBrand = Readonly<{
  paused: boolean;
  company_name: string;
  logo_url?: string | null;
}>;

// What the shell's head says about the product: its icons, install name and preview tags.
const SHELL_BRANDING = [
  'link[rel~="icon"]',
  'link[rel="apple-touch-icon"]',
  'link[rel="manifest"]',
  'meta[name="apple-mobile-web-app-title"]',
  'meta[name="description"]',
  'meta[property^="og:"]',
  'meta[name^="twitter:"]',
].join(",");

// A guest's tab and preview tags name the booking and the host's company, as the
// booking card does. A paused page names no company. The shell's head comes back
// when the screen leaves; null leaves it alone for the host's own views.
export function useBookingTitle(title: string | null, brand?: BookingBrand) {
  const live = brand !== undefined && !brand.paused;
  const company = live ? brand.company_name : "";
  const logo = (live && brand.logo_url) || "";
  useEffect(() => {
    if (title === null) return;
    return replaceShellHead(bookingHead(title, company, logo));
  }, [title, company, logo]);
}

function bookingHead(title: string, company: string, logo: string) {
  const meta = (key: "name" | "property", id: string, content: string) => {
    const tag = document.createElement("meta");
    tag.setAttribute(key, id);
    tag.content = content;
    return tag;
  };
  const icon = document.createElement("link");
  icon.rel = "icon";
  // A blank icon, so the browser never falls back to the app's /favicon.ico.
  icon.href = logo || "data:,";
  const tags: HTMLElement[] = [
    icon,
    meta("property", "og:title", title),
    meta("name", "twitter:card", "summary"),
    meta("name", "twitter:title", title),
  ];
  if (company) tags.push(meta("property", "og:site_name", company));
  if (logo)
    tags.push(
      meta("property", "og:image", logo),
      meta("property", "og:image:alt", company),
      meta("name", "twitter:image", logo),
    );
  return { title, tags };
}

function replaceShellHead(
  head: Readonly<{ title: string; tags: HTMLElement[] }>,
) {
  const previousTitle = document.title;
  const shell = Array.from(
    document.head.querySelectorAll(SHELL_BRANDING),
    (node) => ({ node, next: node.nextSibling }),
  );
  for (const { node } of shell) node.remove();
  document.head.append(...head.tags);
  document.title = head.title;
  return () => {
    for (const tag of head.tags) tag.remove();
    for (const { node, next } of shell.reverse())
      document.head.insertBefore(
        node,
        next?.parentNode === document.head ? next : null,
      );
    document.title = previousTitle;
  };
}

export function BookingZone({
  value,
  onChange,
}: Readonly<{ value: string; onChange: (zone: string) => void }>) {
  const t = useT();
  return (
    <Field label={t("scheduling.timezone")}>
      {(control) => (
        <TimezoneSelect {...control} value={value} onChange={onChange} />
      )}
    </Field>
  );
}
export function useBookingIntent() {
  const intent = useRef<{ payload: string; key: string } | null>(null);
  return (payload: unknown) => {
    const encoded = JSON.stringify(payload);
    if (intent.current?.payload !== encoded)
      intent.current = { payload: encoded, key: crypto.randomUUID() };
    return intent.current.key;
  };
}

export function BookingBack() {
  const t = useT();
  return (
    <div className="book-actions">
      <Button onClick={() => navigate({ screen: "home" })}>
        {t("scheduling.back")}
      </Button>
    </div>
  );
}
