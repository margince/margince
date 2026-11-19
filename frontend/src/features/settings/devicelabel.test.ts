import { expect, it } from "vitest";
import { deviceLabel } from "./devicelabel";

const CHROME_MAC =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36";
const EDGE_WINDOWS =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0";
const SAFARI_IPHONE =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1";
const FIREFOX_LINUX =
  "Mozilla/5.0 (X11; Linux x86_64; rv:127.0) Gecko/20100101 Firefox/127.0";
const CHROME_ANDROID =
  "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36";

it.each([
  [CHROME_MAC, { browser: "Chrome", system: "macOS" }],
  [EDGE_WINDOWS, { browser: "Edge", system: "Windows" }],
  [SAFARI_IPHONE, { browser: "Safari", system: "iOS" }],
  [FIREFOX_LINUX, { browser: "Firefox", system: "Linux" }],
  [CHROME_ANDROID, { browser: "Chrome", system: "Android" }],
])("names the browser and system in %s", (agent, expected) => {
  expect(deviceLabel(agent)).toEqual(expected);
});

it("names nothing for an agent that says nothing", () => {
  expect(deviceLabel(null)).toEqual({});
  expect(deviceLabel("curl/8.4.0")).toEqual({
    browser: undefined,
    system: undefined,
  });
});
