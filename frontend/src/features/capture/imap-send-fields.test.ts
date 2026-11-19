import { describe, expect, it } from "vitest";
import { READ_ONLY, smtpRequest } from "./imap-send-fields";

describe("smtpRequest", () => {
  it("sends nothing while sending is off, whatever was typed", () => {
    expect(
      smtpRequest({ ...READ_ONLY, host: "smtp.example.org", port: "587" }),
    ).toBeUndefined();
  });

  it("defaults the port to the one that goes with the security", () => {
    const on = { ...READ_ONLY, on: true };
    expect(smtpRequest(on)).toEqual({ port: 465, security: "tls" });
    expect(smtpRequest({ ...on, security: "starttls" })).toEqual({
      port: 587,
      security: "starttls",
    });
  });
});
