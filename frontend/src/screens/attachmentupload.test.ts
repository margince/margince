import { afterEach, describe, expect, it, vi } from "vitest";
import { uploadAttachment } from "./attachmentupload";
import { ProblemError } from "./common";

// One upload wrapper for every surface that files bytes, so what the document
// dialog sends and what the contract form sends can no longer drift. These pin
// the parts of the request each caller depends on: the parent, the filing's own
// extra parts, and what a caller is told when the bytes land but the answer
// cannot be read.

const FILE = new File(["signed"], "agreement.pdf", { type: "application/pdf" });
const ORG = { entityType: "organization", entityId: "org-1" } as const;

/** The one request the wrapper made, recorded as it was sent. */
type Sent = { url: string; credentials?: RequestCredentials; parts: FormData };

/** Answers every upload with `answer`, and keeps what was sent. */
function recordingFetch(answer: () => Response) {
  const sent: Sent[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      if (!(init?.body instanceof FormData)) {
        throw new Error("the upload did not send a multipart body");
      }
      sent.push({
        url: String(input),
        credentials: init.credentials,
        parts: init.body,
      });
      return answer();
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return sent;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("uploadAttachment", () => {
  it("sends the parent and the bytes, and returns the stored document", async () => {
    const sent = recordingFetch(() => Response.json({ id: "a-9" }));

    const stored = await uploadAttachment(ORG, FILE);

    expect(stored?.id).toBe("a-9");
    expect(sent[0].url).toBe("/v1/attachments");
    // The session cookie is what authorizes the upload; a request that omitted
    // it would be refused as anonymous.
    expect(sent[0].credentials).toBe("include");
    expect(sent[0].parts.get("entity_type")).toBe("organization");
    expect(sent[0].parts.get("entity_id")).toBe("org-1");
    expect(sent[0].parts.get("file")).toBe(FILE);
  });

  it("appends a filing's extra parts before the bytes", async () => {
    const sent = recordingFetch(() => Response.json({ id: "a-9" }));

    await uploadAttachment(ORG, FILE, { contract_id: "c-1" });

    expect(sent[0].parts.get("contract_id")).toBe("c-1");
    // Order, because the file is documented as the last part: a reader of the
    // body as a stream meets the small fields before the bytes.
    expect([...sent[0].parts.keys()]).toEqual([
      "entity_type",
      "entity_id",
      "contract_id",
      "file",
    ]);
  });

  it("throws the server's own refusal rather than a bare status", async () => {
    recordingFetch(() =>
      Response.json(
        { title: "too_large", detail: "That file is over the limit." },
        { status: 413 },
      ),
    );

    await expect(uploadAttachment(ORG, FILE)).rejects.toThrow(ProblemError);
    await expect(uploadAttachment(ORG, FILE)).rejects.toThrow(
      "That file is over the limit.",
    );
  });

  it("reports no document, not a failure, when an accepted upload answers nothing readable", async () => {
    // The bytes are stored — the status said so. A caller learns it has no id
    // to write metadata against, which is a different fact from a refusal and
    // the reason this does not throw.
    recordingFetch(() => new Response("", { status: 201 }));

    await expect(uploadAttachment(ORG, FILE)).resolves.toBeUndefined();
  });
});
