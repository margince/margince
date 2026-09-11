// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The refusal must cost the SERVER nothing, which is a claim about order and
// not about the status code.
//
// The store's gate is the one that decides, and it decides after the multipart
// parse: the object type arrives inside the body. So a session holding no write
// grant anywhere used to spend one request and make this server parse and spill
// a whole file before answering 403 — repeatable, and free to the caller. These
// hold the early refusal in front of that, in BOTH directions: a caller it turns
// away never had its body read, and a caller it admits is not turned away.

// watchedBody reports whether anything read it. A bare flag is enough: the
// question is whether the parse ran at all, not how much of it did.
type watchedBody struct {
	r    io.Reader
	read bool
}

func (b *watchedBody) Read(p []byte) (int, error) {
	b.read = true
	return b.r.Read(p)
}

func uploadRequest(t *testing.T, p principal.Principal) (*httptest.ResponseRecorder, *watchedBody) {
	t.Helper()
	body := &watchedBody{r: strings.NewReader("--x\r\nnot a well-formed part\r\n--x--\r\n")}
	req := httptest.NewRequest(http.MethodPost, "/v1/attachments", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req = req.WithContext(principal.WithActor(req.Context(), p))
	rec := httptest.NewRecorder()
	Handlers{}.WithUploadLimit(25<<20).UploadAttachment(rec, req)
	return rec, body
}

func TestAnUploadFromAGrantlessSessionIsRefusedBeforeItsBodyIsRead(t *testing.T) {
	rec, body := uploadRequest(t, principal.Principal{
		Type: principal.PrincipalHuman,
		// A reader: every grant this session holds is a read, so no object type
		// the body could name would admit the write.
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{
			"contact":  {Read: true},
			"activity": {Read: true},
		}},
	})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("a session with no write grant anywhere got %d, want 403", rec.Code)
	}
	if body.read {
		t.Error("the refusal read the body first, which is the whole cost it exists to avoid: " +
			"the grant check has to run in front of the multipart parse, not behind it")
	}
}

func TestAnUploadFromASessionThatMayWriteSomethingReachesTheParse(t *testing.T) {
	// The early check is deliberately coarse — it cannot know which object the
	// body names — so its one failure mode is refusing a caller the store would
	// have admitted. This is that direction: a grant on ANY object type is
	// enough to get past it, and the body is read.
	rec, body := uploadRequest(t, principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true, Update: true},
		}},
	})

	if !body.read {
		t.Fatal("a session holding a write grant never reached the parse, so the early " +
			"refusal is turning away callers the store would admit")
	}
	if rec.Code == http.StatusForbidden {
		t.Errorf("the early check refused a session that may write: %s", rec.Body.String())
	}
}
