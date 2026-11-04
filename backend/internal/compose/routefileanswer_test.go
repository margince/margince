// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func recorded(contentType, disposition string, body []byte) *cappedRecorder {
	rec := &cappedRecorder{ResponseRecorder: httptest.NewRecorder(), fileLimit: agents.MaxInlineFileBytes}
	rec.Header().Set("Content-Type", contentType)
	if disposition != "" {
		rec.Header().Set("Content-Disposition", disposition)
	}
	rec.WriteHeader(http.StatusOK)
	if _, err := rec.Write(body); err != nil {
		panic(err)
	}
	return rec
}

func TestARouteThatAnswersAFileAnswersItsBytesAndName(t *testing.T) {
	content := []byte{0x25, 0x50, 0x44, 0x46, 0x00, 0xff}
	raw, err := routeAnswer(agents.RouteVerb{Verb: "download_offer_pdf", Method: http.MethodGet}, recorded("application/pdf", `attachment; filename="offer.pdf"`, content))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Filename      string `json:"filename"`
		ContentType   string `json:"content_type"`
		SizeBytes     int    `json:"size_bytes"`
		ContentBase64 string `json:"content_base64"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.ContentBase64)
	if err != nil {
		t.Fatal(err)
	}
	if got.Filename != "offer.pdf" || got.ContentType != "application/pdf" || got.SizeBytes != len(content) || !bytes.Equal(decoded, content) {
		t.Fatalf("answer = %+v, decoded %v", got, decoded)
	}
}

func TestAnEmptyFileIsStillAFile(t *testing.T) {
	raw, err := routeAnswer(agents.RouteVerb{Verb: "download_attachment", Method: http.MethodGet}, recorded("text/csv", "", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"size_bytes":0`) || !strings.Contains(string(raw), `"content_base64":""`) {
		t.Fatalf("an empty file answered %s", raw)
	}
}

func TestAFilePastTheLimitIsRefusedNotCut(t *testing.T) {
	raw, err := routeAnswer(agents.RouteVerb{Verb: "download_attachment", Method: http.MethodGet}, recorded("application/pdf", "", make([]byte, agents.MaxInlineFileBytes+1)))
	if err == nil {
		t.Fatalf("an oversized file answered %d bytes instead of a refusal", len(raw))
	}
	if !strings.Contains(err.Error(), httperr.Megabytes(agents.MaxInlineFileBytes)) {
		t.Fatalf("the refusal does not state the limit: %v", err)
	}
}

func TestAFileAtTheLimitIsAnsweredWhole(t *testing.T) {
	raw, err := routeAnswer(agents.RouteVerb{Verb: "download_attachment", Method: http.MethodGet}, recorded("application/pdf", "", make([]byte, agents.MaxInlineFileBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > agents.MaxMCPRequestBytes {
		t.Fatalf("a file at the limit took %d bytes, past the %d one message carries", len(raw), agents.MaxMCPRequestBytes)
	}
}

func TestAWritesJSONAnswerPastItsCapStaysACutSuccess(t *testing.T) {
	raw, err := routeAnswer(agents.RouteVerb{Verb: "make_thing", Method: http.MethodPost}, recorded("application/json", "", make([]byte, maxViewAnswerBytes+1)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"truncated":true`) {
		t.Fatalf("answer = %s", raw)
	}
}

// The recorder sniffs a type for a body whose handler declared none. That guess
// does not make the body a file, so the caller reads it as text.
func TestABodyWithNoDeclaredTypeIsNotAnsweredAsAFile(t *testing.T) {
	rec := &cappedRecorder{ResponseRecorder: httptest.NewRecorder(), fileLimit: agents.MaxInlineFileBytes}
	if _, err := rec.Write([]byte("plain words from a handler")); err != nil {
		t.Fatal(err)
	}
	raw, err := routeAnswer(agents.RouteVerb{Verb: "list_things", Method: http.MethodGet}, rec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "content_base64") || !strings.Contains(string(raw), "plain words from a handler") {
		t.Fatalf("answer = %s, want the words readable", raw)
	}
}

// A handler that declares a file and writes no body still answers a file.
func TestAFileTypeDeclaredWithNoBodyIsStillAFile(t *testing.T) {
	rec := &cappedRecorder{ResponseRecorder: httptest.NewRecorder(), fileLimit: agents.MaxInlineFileBytes}
	rec.Header().Set("Content-Type", "text/csv")
	rec.WriteHeader(http.StatusOK)
	raw, err := routeAnswer(agents.RouteVerb{Verb: "download_attachment", Method: http.MethodGet}, rec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"content_base64":""`) {
		t.Fatalf("answer = %s, want an empty file", raw)
	}
}

// A route this installation does not serve answers 501, which no retry changes,
// so the caller is told that rather than that the server failed.
func TestARouteThatIsNotOfferedHereSaysSoAndAServerFaultDoesNot(t *testing.T) {
	answered := func(status int) *cappedRecorder {
		rec := &cappedRecorder{ResponseRecorder: httptest.NewRecorder(), fileLimit: agents.MaxInlineFileBytes}
		rec.Header().Set("Content-Type", "application/problem+json")
		rec.WriteHeader(status)
		if _, err := rec.Write([]byte(`{"code":"not_implemented","detail":"no connector is configured"}`)); err != nil {
			t.Fatal(err)
		}
		return rec
	}
	verb := agents.RouteVerb{Verb: "list_connectors", Method: http.MethodGet}
	_, err := routeAnswer(verb, answered(http.StatusNotImplemented))
	fault, classified := httperr.Classify(err)
	if !classified || fault.Status != http.StatusNotImplemented || fault.Detail != "no connector is configured" {
		t.Fatalf("a 501 answered %v (classified %v, %+v), want the route's own 501", err, classified, fault)
	}
	_, err = routeAnswer(verb, answered(http.StatusInternalServerError))
	if _, classified := httperr.Classify(err); classified {
		t.Fatalf("a 500 answered %v as a classified refusal, want a fault of the server", err)
	}
}
