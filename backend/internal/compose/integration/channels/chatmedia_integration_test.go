// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// The files of WhatsApp and Zalo messages, from the webhook to History. A local
// TLS server plays the Graph API and both networks' file hosts. Everything after
// the download runs for real: the ingest worker, the sink, the file keeper and
// the eraser.

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/chatedge"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var (
	pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), []byte("IHDR image bytes")...)
	pdfBytes = []byte("%PDF-1.4 the offer")
)

// fakeFile is how the local server answers for one file.
type fakeFile struct {
	body []byte
	// metaSize overrides the size the Graph metadata states.
	metaSize int64
	// status, when set, answers the download itself.
	status int
	// failFirst answers this many downloads with 503 before the real one.
	failFirst int
	redirect  string
}

// mediaFake is the Graph API and the file hosts of both networks.
type mediaFake struct {
	srv   *httptest.Server
	mu    sync.Mutex
	files map[string]*fakeFile
	hits  map[string]int
	auth  map[string]string
}

func startMediaFake(t *testing.T) *mediaFake {
	t.Helper()
	f := &mediaFake{files: map[string]*fakeFile{}, hits: map[string]int{}, auth: map[string]string{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *mediaFake) add(name string, file fakeFile) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[name] = &file
	return f.srv.URL + "/dl/" + name
}

func (f *mediaFake) hitsOf(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[name]
}

func (f *mediaFake) authOf(name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.auth[name]
}

func (f *mediaFake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if name, ok := strings.CutPrefix(r.URL.Path, "/"+whatsapp.GraphVersion+"/"); ok {
		file := f.files[name]
		if file == nil {
			http.Error(w, `{"error":{"message":"gone","code":100}}`, http.StatusNotFound)
			return
		}
		f.auth["meta:"+name] = r.Header.Get("Authorization")
		sum := sha256.Sum256(file.body)
		size := int64(len(file.body))
		if file.metaSize != 0 {
			size = file.metaSize
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"url": f.srv.URL + "/dl/" + name, "mime_type": "application/octet-stream",
			"sha256": hex.EncodeToString(sum[:]), "file_size": size,
		}); err != nil {
			http.Error(w, "encode", http.StatusInternalServerError)
		}
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/dl/")
	file := f.files[name]
	if file == nil {
		http.NotFound(w, r)
		return
	}
	f.hits[name]++
	f.auth[name] = r.Header.Get("Authorization")
	switch {
	case file.redirect != "":
		http.Redirect(w, r, file.redirect, http.StatusFound)
	case f.hits[name] <= file.failFirst:
		http.Error(w, "try later", http.StatusServiceUnavailable)
	case file.status != 0:
		http.Error(w, "refused", file.status)
	default:
		if _, err := w.Write(file.body); err != nil {
			return
		}
	}
}

func (f *mediaFake) clients() chatedge.MediaClients {
	return chatedge.MediaClients{
		WhatsApp: whatsapp.NewClient(f.srv.Client(), f.srv.URL, "", whatsapp.WithMediaHosts("127.0.0.1")),
		Zalo:     zalo.NewClient(f.srv.Client(), "", "", zalo.WithMediaHosts("127.0.0.1")),
	}
}

// mediaWorker boots the worker with the fake's clients and a real object store.
func mediaWorker(t *testing.T, c *telegramEnv, f *mediaFake) (blobstore.Store, func() <-chan *river.Event) {
	t.Helper()
	blob := blobstore.NewMemory()
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{
		ChatMedia:     f.clients(),
		CaptureConfig: compose.CaptureConfig{Blob: blob},
	})
	startTelegramWorker(t, runner)
	return blob, func() <-chan *river.Event { return sub }
}

// shareNumber lets colleagues read the connection's messages, so a file is kept
// with its bytes.
func (c *telegramEnv) shareConnection(t *testing.T, kind, id string) {
	t.Helper()
	c.allowSharedPosture(t)
	if status := c.Call(t, http.MethodPatch, "/v1/channel-connections/"+kind+"/"+id+"/visibility",
		integration.AnyMap{"mail_visibility": "shared", "confirm": true}, nil, nil); status != http.StatusOK {
		t.Fatalf("sharing %s %s → %d, want 200", kind, id, status)
	}
}

// storedFile is what History's attachment row says about one kept file.
type storedFile struct {
	filename, contentType string
	bytes                 int64
	withheld              bool
}

func (c *telegramEnv) filesOf(t *testing.T, system, messageID string) []storedFile {
	t.Helper()
	var out []storedFile
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT filename, coalesce(content_type, ''), coalesce(byte_size, 0), bytes_withheld
			  FROM attachment WHERE external_source_id LIKE $1 ORDER BY external_part_id`, system+":%:"+messageID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var f storedFile
			if err := rows.Scan(&f.filename, &f.contentType, &f.bytes, &f.withheld); err != nil {
				return err
			}
			out = append(out, f)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the kept files: %v", err)
	}
	return out
}

// dropsOf counts the files one reason refused, as the capture breadcrumb states.
func (c *telegramEnv) dropsOf(t *testing.T, reason string) int {
	return c.count(t, `SELECT coalesce(sum((detail->>'dropped_parts')::int), 0)::int FROM system_log
		WHERE action = 'capture_parts_dropped' AND detail->>'reason' = $1`, reason)
}

func (c *telegramEnv) erase(t *testing.T, contact string, blob blobstore.Store) {
	t.Helper()
	id, err := ids.Parse(contact)
	if err != nil {
		t.Fatal(err)
	}
	if err := privacy.NewEraser(c.DB()).WithBlobstore(blob).EraseContact(c.eraserCtx(t), id, "test"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}
}

func sha256B64(b []byte) string {
	sum := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func waMedia(phoneID, wamid, typ, body string) string {
	return waChange("messages", phoneID, waAda, "messages",
		fmt.Sprintf(`{"from":"491701234567","id":%q,"timestamp":"1764000300","type":%q,%q:%s}`, wamid, typ, typ, body))
}

func waEchoMedia(phoneID, wamid, typ, body string) string {
	return waChange("smb_message_echoes", phoneID, waAda, "message_echoes",
		fmt.Sprintf(`{"from":"15550001111","to":"491701234567","id":%q,"timestamp":"1764000400","type":%q,%q:%s}`, wamid, typ, typ, body))
}

func TestAWhatsAppImageAndAnEchoedDocumentReachHistoryAsFiles(t *testing.T) {
	c := setupWhatsApp(t)
	f := startMediaFake(t)
	f.add("IMG1", fakeFile{body: pngBytes})
	docURL := f.add("DOC1", fakeFile{body: pdfBytes})
	_, sub := mediaWorker(t, c, f)
	n := c.connectNumber(t, "7201")
	c.shareConnection(t, "whatsapp", n.id)

	c.deliverWA(t, n, waMedia(n.phoneID, "wamid.IMG", "image",
		fmt.Sprintf(`{"id":"IMG1","mime_type":"image/jpeg","sha256":%q,"caption":"the contract"}`, sha256B64(pngBytes))))
	awaitWA(t, sub())
	c.deliverWA(t, n, waEchoMedia(n.phoneID, "wamid.DOC", "document",
		fmt.Sprintf(`{"url":%q,"mime_type":"application/pdf","filename":"offer.pdf"}`, docURL)))
	awaitWA(t, sub())

	img := c.filesOf(t, "whatsapp", "wamid.IMG")
	if len(img) != 1 || img[0].contentType != "image/png" || img[0].bytes != int64(len(pngBytes)) || img[0].withheld {
		t.Errorf("the image kept as %+v, want one png of %d bytes with its bytes", img, len(pngBytes))
	}
	doc := c.filesOf(t, "whatsapp", "wamid.DOC")
	if len(doc) != 1 || doc[0].filename != "offer.pdf" || doc[0].contentType != "application/pdf" {
		t.Errorf("the echoed document kept as %+v, want offer.pdf as application/pdf", doc)
	}
	if got := f.authOf("meta:IMG1"); got != "Bearer EAAG-7201" {
		t.Errorf("the media metadata call sent %q, want the number's access token", got)
	}
	if got := c.count(t, `SELECT count(*) FROM activity WHERE source_system = 'whatsapp' AND body = 'the contract'`); got != 1 {
		t.Errorf("%d messages carry the caption as their text, want 1", got)
	}

	c.deliverWA(t, n, waMedia(n.phoneID, "wamid.IMG", "image", `{"id":"IMG1","mime_type":"image/jpeg"}`))
	if got := len(c.filesOf(t, "whatsapp", "wamid.IMG")); got != 1 {
		t.Errorf("after a redelivery %d files, want the one copy", got)
	}
}

func TestAWhatsAppFileThatCannotBeKeptIsCountedAndTheMessageStillFiles(t *testing.T) {
	cases := []struct {
		name   string
		file   fakeFile
		reason string
	}{
		{"larger than the limit", fakeFile{body: pdfBytes, metaSize: 30 << 20}, chatedge.DropTooLarge},
		{"gone at the provider", fakeFile{body: pdfBytes, status: http.StatusNotFound}, chatedge.DropRefused},
		{"leaves the allowed hosts", fakeFile{body: pdfBytes, redirect: "https://elsewhere.example.test/x"}, chatedge.DropRefused},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := setupWhatsApp(t)
			f := startMediaFake(t)
			f.add("M", tc.file)
			_, sub := mediaWorker(t, c, f)
			n := c.connectNumber(t, fmt.Sprintf("73%02d", i))
			c.shareConnection(t, "whatsapp", n.id)
			before := c.dropsOf(t, tc.reason)

			c.deliverWA(t, n, waMedia(n.phoneID, "wamid.BAD", "document", `{"id":"M","mime_type":"application/pdf","filename":"a.pdf"}`))
			awaitWA(t, sub())

			if _, filed := c.waFiled(t, "wamid.BAD"); !filed {
				t.Fatal("the message was not filed")
			}
			if files := c.filesOf(t, "whatsapp", "wamid.BAD"); len(files) != 0 {
				t.Errorf("kept %+v, want no file", files)
			}
			if got := c.dropsOf(t, tc.reason) - before; got != 1 {
				t.Errorf("%d files counted as %q, want 1", got, tc.reason)
			}
		})
	}
}

func TestAWhatsAppFileThatIsTemporarilyUnavailableIsRetriedAndKeptOnce(t *testing.T) {
	c := setupWhatsApp(t)
	f := startMediaFake(t)
	f.add("M", fakeFile{body: pdfBytes, failFirst: 1})
	_, sub := mediaWorker(t, c, f)
	n := c.connectNumber(t, "7401")
	c.shareConnection(t, "whatsapp", n.id)

	c.deliverWA(t, n, waMedia(n.phoneID, "wamid.RETRY", "document", `{"id":"M","mime_type":"application/pdf","filename":"a.pdf"}`))
	awaitWA(t, sub())

	if got := f.hitsOf("M"); got != 2 {
		t.Errorf("the file was fetched %d times, want the failed attempt and its retry", got)
	}
	if files := c.filesOf(t, "whatsapp", "wamid.RETRY"); len(files) != 1 {
		t.Errorf("kept %d files, want 1", len(files))
	}
}

func TestErasingAWhatsAppContactRemovesTheFilesOfItsMessages(t *testing.T) {
	c := setupWhatsApp(t)
	f := startMediaFake(t)
	f.add("M", fakeFile{body: pdfBytes})
	blob, sub := mediaWorker(t, c, f)
	n := c.connectNumber(t, "7501")
	c.shareConnection(t, "whatsapp", n.id)
	c.deliverWA(t, n, waMedia(n.phoneID, "wamid.ERASE", "document", `{"id":"M","mime_type":"application/pdf","filename":"a.pdf"}`))
	awaitWA(t, sub())
	filed, _ := c.waFiled(t, "wamid.ERASE")
	if len(c.filesOf(t, "whatsapp", "wamid.ERASE")) != 1 {
		t.Fatal("the file was not kept before the erasure")
	}

	c.erase(t, filed.contact, blob)

	if files := c.filesOf(t, "whatsapp", "wamid.ERASE"); len(files) != 0 {
		t.Errorf("%d files survive the erasure, want none", len(files))
	}
}

func TestAFileFollowsTheAudienceOfItsWhatsAppMessage(t *testing.T) {
	c := setupWhatsApp(t)
	f := startMediaFake(t)
	f.add("M", fakeFile{body: pdfBytes})
	blob, sub := mediaWorker(t, c, f)
	n := c.connectNumber(t, "7601")
	store := activities.NewStore(c.DB()).WithBlobstore(blob)
	stranger := c.strangerRepCtx(t, map[string]principal.ObjectGrant{"activity": {Read: true}})
	doc := `{"id":"M","mime_type":"application/pdf","filename":"a.pdf"}`

	c.deliverWA(t, n, waMedia(n.phoneID, "wamid.HELD", "document", doc))
	awaitWA(t, sub())
	held, _ := c.waFiled(t, "wamid.HELD")
	if _, _, err := store.ListAttachments(stranger, "activity", mustParse(t, held.id), nil, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a colleague listing the file of a held message got %v, want not found", err)
	}

	c.shareConnection(t, "whatsapp", n.id)
	c.deliverWA(t, n, waMedia(n.phoneID, "wamid.SHARED", "document", doc))
	awaitWA(t, sub())
	shared, _ := c.waFiled(t, "wamid.SHARED")
	if got, _, err := store.ListAttachments(stranger, "activity", mustParse(t, shared.id), nil, nil); err != nil || len(got) != 1 {
		t.Errorf("a colleague listing the file of a shared message got %d files and %v, want 1", len(got), err)
	}
}

func mustParse(t *testing.T, id string) ids.UUID {
	t.Helper()
	parsed, err := ids.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func zaloMedia(oaID, msgID, event, text, attachments string) string {
	sender, recipient := zaloCustomer, oaID
	if strings.HasPrefix(event, "oa_") {
		sender, recipient = oaID, zaloCustomer
	}
	return fmt.Sprintf(`{"app_id":%q,"event_name":%q,"timestamp":%q,"sender":{"id":%q},"recipient":{"id":%q},"message":{"msg_id":%q,"text":%q,"attachments":[%s]}}`,
		zaloAppID, event, zaloTimestamp, sender, recipient, msgID, text, attachments)
}

func zaloAttachment(typ, url, name string, size int64, checksum string) string {
	return fmt.Sprintf(`{"type":%q,"payload":{"url":%q,"name":%q,"size":%d,"checksum":%q}}`, typ, url, name, size, checksum)
}

func md5Hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}

func TestAZaloImageAndFileReachHistoryAsFiles(t *testing.T) {
	c := setupZalo(t)
	f := startMediaFake(t)
	imgURL := f.add("img.png", fakeFile{body: pngBytes})
	fileURL := f.add("offer.pdf", fakeFile{body: pdfBytes})
	_, sub := mediaWorker(t, c, f)
	oa := c.connectOA(t, "9201")
	c.shareConnection(t, "zalo-oa", oa.id)

	c.deliverZalo(t, oa, zaloMedia(oa.oaID, "zm-img", "user_send_image", "ảnh", zaloAttachment("image", imgURL, "", 0, "")))
	awaitZalo(t, sub())
	c.deliverZalo(t, oa, zaloMedia(oa.oaID, "zm-file", "user_send_file", "",
		`{"type":"link","payload":{"url":"https://example.test/page"}},`+zaloAttachment("file", fileURL, "bao-gia.pdf", int64(len(pdfBytes)), md5Hex(pdfBytes))))
	awaitZalo(t, sub())

	img := c.filesOf(t, "zalo_oa", "zm-img")
	if len(img) != 1 || img[0].contentType != "image/png" || img[0].withheld {
		t.Errorf("the image kept as %+v, want one png with its bytes", img)
	}
	file := c.filesOf(t, "zalo_oa", "zm-file")
	if len(file) != 1 || file[0].filename != "bao-gia.pdf" || file[0].contentType != "application/pdf" {
		t.Errorf("the file kept as %+v, want bao-gia.pdf as application/pdf", file)
	}
	if got := c.count(t, `SELECT count(*) FROM activity WHERE source_system = 'zalo_oa' AND body = 'ảnh'`); got != 1 {
		t.Errorf("%d messages carry the caption as their text, want 1", got)
	}
	if got := f.authOf("img.png"); got != "" {
		t.Errorf("the file host was sent %q, want no credential", got)
	}
	c.deliverZalo(t, oa, zaloMedia(oa.oaID, "zm-img", "user_send_image", "ảnh", zaloAttachment("image", imgURL, "", 0, "")))
	if got := len(c.filesOf(t, "zalo_oa", "zm-img")); got != 1 {
		t.Errorf("after a redelivery %d files, want the one copy", got)
	}
}

func TestAZaloFileThatCannotBeKeptIsCountedAndTheMessageStillFiles(t *testing.T) {
	cases := []struct {
		name     string
		file     fakeFile
		size     int64
		checksum string
		reason   string
	}{
		{"larger than the limit", fakeFile{body: pdfBytes}, 30 << 20, "", chatedge.DropTooLarge},
		{"gone at the host", fakeFile{body: pdfBytes, status: http.StatusNotFound}, 0, "", chatedge.DropRefused},
		{"leaves the allowed hosts", fakeFile{body: pdfBytes, redirect: "https://elsewhere.example.test/x"}, 0, "", chatedge.DropRefused},
		{"differs from its checksum", fakeFile{body: pdfBytes}, 0, md5Hex([]byte("other")), chatedge.DropChecksum},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := setupZalo(t)
			f := startMediaFake(t)
			url := f.add("bad.pdf", tc.file)
			_, sub := mediaWorker(t, c, f)
			oa := c.connectOA(t, fmt.Sprintf("93%02d", i))
			c.shareConnection(t, "zalo-oa", oa.id)
			before := c.dropsOf(t, tc.reason)

			c.deliverZalo(t, oa, zaloMedia(oa.oaID, "zm-bad", "user_send_file", "", zaloAttachment("file", url, "a.pdf", tc.size, tc.checksum)))
			awaitZalo(t, sub())

			if _, filed := c.zaloFiled(t, "zm-bad"); !filed {
				t.Fatal("the message was not filed")
			}
			if files := c.filesOf(t, "zalo_oa", "zm-bad"); len(files) != 0 {
				t.Errorf("kept %+v, want no file", files)
			}
			if got := c.dropsOf(t, tc.reason) - before; got != 1 {
				t.Errorf("%d files counted as %q, want 1", got, tc.reason)
			}
		})
	}
}

func TestAZaloFileThatIsTemporarilyUnavailableIsRetriedAndKeptOnce(t *testing.T) {
	c := setupZalo(t)
	f := startMediaFake(t)
	url := f.add("slow.pdf", fakeFile{body: pdfBytes, failFirst: 1})
	_, sub := mediaWorker(t, c, f)
	oa := c.connectOA(t, "9401")
	c.shareConnection(t, "zalo-oa", oa.id)

	c.deliverZalo(t, oa, zaloMedia(oa.oaID, "zm-retry", "user_send_file", "", zaloAttachment("file", url, "a.pdf", 0, "")))
	awaitZalo(t, sub())

	if got := f.hitsOf("slow.pdf"); got != 2 {
		t.Errorf("the file was fetched %d times, want the failed attempt and its retry", got)
	}
	if files := c.filesOf(t, "zalo_oa", "zm-retry"); len(files) != 1 {
		t.Errorf("kept %d files, want 1", len(files))
	}
}

func TestErasingAZaloContactRemovesTheFilesOfItsMessages(t *testing.T) {
	c := setupZalo(t)
	f := startMediaFake(t)
	url := f.add("e.pdf", fakeFile{body: pdfBytes})
	blob, sub := mediaWorker(t, c, f)
	oa := c.connectOA(t, "9501")
	c.shareConnection(t, "zalo-oa", oa.id)
	c.deliverZalo(t, oa, zaloMedia(oa.oaID, "zm-erase", "user_send_file", "", zaloAttachment("file", url, "a.pdf", 0, "")))
	awaitZalo(t, sub())
	filed, _ := c.zaloFiled(t, "zm-erase")
	if len(c.filesOf(t, "zalo_oa", "zm-erase")) != 1 {
		t.Fatal("the file was not kept before the erasure")
	}

	c.erase(t, filed.contact, blob)

	if files := c.filesOf(t, "zalo_oa", "zm-erase"); len(files) != 0 {
		t.Errorf("%d files survive the erasure, want none", len(files))
	}
}
