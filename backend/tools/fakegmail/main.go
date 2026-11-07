// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command fakegmail serves demo mailboxes over the part of the Gmail API and
// Google OAuth that the Gmail connector calls. Several history imports can then
// run through a real worker on one machine; docs/how-to/measure-mail-import.md
// shows how to point a dev stack at it.
//
//	go run ./tools/fakegmail -owners admin@demo.test,rep@demo.test -messages 20000 -big
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18085", "listen address")
	owners := flag.String("owners", "admin@demo.test,rep@demo.test,rep2@demo.test", "mailbox owners, comma separated")
	count := flag.Int("messages", 20000, "messages per mailbox")
	big := flag.Bool("big", false, "attachments of realistic size, up to 15 MiB")
	generation := flag.Int("generation", 0, "serve the mailboxes as new mail, for a second load run on one database")
	callbackOrigin := flag.String("callback-origin", "", "one more origin the consent page may redirect to, such as https://app.test, when the stack's public URL is not on loopback")
	flag.Parse()

	s := &server{byOwner: map[string]int{}, connected: map[string]bool{}, now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), big: *big, generation: *generation, callbackOrigin: strings.ToLower(strings.TrimSuffix(*callbackOrigin, "/"))}
	for i, owner := range strings.Split(*owners, ",") {
		digest := sha256.Sum256([]byte(owner))
		s.boxes = append(s.boxes, mailbox{owner: owner, count: *count, seed: binary.LittleEndian.Uint64(digest[:])})
		s.byOwner[owner] = i
	}
	log.Printf("fakegmail on %s: %d mailboxes of %d messages", *addr, len(s.boxes), *count)
	srv := &http.Server{Addr: *addr, Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
