// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imap

// The folder listing against a real (in-memory) IMAP server. The in-test
// dialer replaces only the transport; LIST onward is the production path.

import (
	"context"
	"errors"
	"net"
	"slices"
	"testing"

	imapv2 "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The listing is the account's real mailboxes, and the server's own hierarchy
// delimiter stays in the name: "INBOX/Privat" is what the owner reads in their
// client, and a tidier name is one no exclusion rule could match.
func TestListContainersReturnsTheAccountsMailboxesWithTheirPaths(t *testing.T) {
	addr, user := startMemServer(t, 1)
	if err := user.Create("INBOX/Privat", nil); err != nil {
		t.Fatal(err)
	}
	c := NewStanding().withDialer(plainDialer(addr))

	got, err := c.ListContainers(context.Background(), standingAuth(t))
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}

	names := make([]string, 0, len(got))
	for _, m := range got {
		// An IMAP mailbox is named by its path, so there is no opaque token to
		// hide behind a label — the id and the name are the same string.
		if m.ID != m.Name {
			t.Errorf("mailbox %+v: id and name should be the same string", m)
		}
		names = append(names, m.Name)
	}
	if !slices.Contains(names, "INBOX") || !slices.Contains(names, "INBOX/Privat") {
		t.Fatalf("listing = %v, want INBOX and the nested mailbox with its delimiter kept", names)
	}
}

// A hierarchy node is not a place mail sits. A \Noselect mailbox cannot be
// opened, so it cannot hold a message to exclude, and offering one would give
// somebody a choice that excludes nothing.
func TestListContainersDropsMailboxesThatCannotBeOpened(t *testing.T) {
	addr := listenNoselect(t)
	c := NewStanding().withDialer(plainDialer(addr))

	got, err := c.ListContainers(context.Background(), standingAuth(t))
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	for _, m := range got {
		if m.Name == noselectNode {
			t.Fatalf("listing offered %q, a node that cannot be selected", m.Name)
		}
	}
	if !slices.Contains(containerNames(got), "INBOX") {
		t.Fatalf("listing = %v, want the selectable mailbox kept", containerNames(got))
	}
}

// A server that refuses the login yields the dial's own error rather than an
// empty listing: "no folders" and "we could not ask" are different answers,
// and the picker tells them apart.
func TestListContainersSurfacesADialFailure(t *testing.T) {
	addr, _ := startMemServer(t, 1)
	c := NewStanding().withDialer(plainDialer(addr))

	creds := standingCreds(t)
	creds.Password = "not-the-password"
	_, err := c.ListContainers(context.Background(), sealCreds(t, creds))
	if !errors.Is(err, ErrLoginRejected) {
		t.Fatalf("err = %v, want the dial's refusal", err)
	}
}

// A LIST the server does not answer is the server not answering. It surfaces
// as ErrUnreachable so the transport above tells "we could not ask your
// mailbox" apart from "your mailbox has no folders" — different facts, and a
// reader acts on them differently.
func TestListContainersMarksAFailedListAsUnreachable(t *testing.T) {
	addr := listenRefusingList(t)
	c := NewStanding().withDialer(plainDialer(addr))

	_, err := c.ListContainers(context.Background(), standingAuth(t))
	if !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("err = %v, want it marked unreachable", err)
	}
}

// listenRefusingList serves a store whose LIST always fails.
func listenRefusingList(t *testing.T) string {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser(memUser, memPass)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	mem.AddUser(user)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &refusingListSession{Session: mem.NewSession()}, nil, nil
		},
		InsecureAuth: true,
	})
	go func() {
		//craft:ignore swallowed-errors the listener closes at test end; Serve's shutdown error is the expected exit
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() {
		//craft:ignore swallowed-errors test-server shutdown; the assertions already ran
		_ = srv.Close()
	})
	return ln.Addr().String()
}

// refusingListSession answers every LIST with a failure.
type refusingListSession struct{ imapserver.Session }

func (s *refusingListSession) List(*imapserver.ListWriter, string, []string, *imapv2.ListOptions) error {
	return errors.New("LIST unavailable")
}

func containerNames(got []connector.NamedContainer) []string {
	out := make([]string, 0, len(got))
	for _, m := range got {
		out = append(out, m.Name)
	}
	return out
}

const noselectNode = "Archive"

// noselectSession is the in-memory session with one hierarchy node added to
// its listing. The memory server never produces a \Noselect mailbox of its
// own, and it is the one thing a real server has that the fixture lacks.
type noselectSession struct{ imapserver.Session }

func (s *noselectSession) List(w *imapserver.ListWriter, ref string, patterns []string, options *imapv2.ListOptions) error {
	if err := s.Session.List(w, ref, patterns, options); err != nil {
		return err
	}
	return w.WriteList(&imapv2.ListData{
		Mailbox: noselectNode,
		Attrs:   []imapv2.MailboxAttr{imapv2.MailboxAttrNoSelect},
		Delim:   '/',
	})
}

// listenNoselect serves one in-memory mailbox store whose listing carries an
// unselectable node beside the real mailbox.
func listenNoselect(t *testing.T) string {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser(memUser, memPass)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	mem.AddUser(user)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &noselectSession{Session: mem.NewSession()}, nil, nil
		},
		InsecureAuth: true,
	})
	go func() {
		//craft:ignore swallowed-errors the listener closes at test end; Serve's shutdown error is the expected exit
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() {
		//craft:ignore swallowed-errors test-server shutdown; the assertions already ran
		_ = srv.Close()
	})
	return ln.Addr().String()
}
