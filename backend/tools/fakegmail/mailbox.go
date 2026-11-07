// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
)

type mailbox struct {
	owner string
	count int
	seed  uint64
}

type message struct {
	id      string
	raw     string
	labels  []string
	date    time.Time
	headers [][2]string
}

// The correspondents are shared by every mailbox, so colleagues write to the
// same customers, and a few customers get most of the mail.
const (
	companies           = 150
	addressesPerCompany = 4
	freemailUsers       = 120
	newsletters         = 30
)

var (
	firstNames = strings.Fields("anna ben clara david eva felix greta hans ida jonas karla lukas mia noah olga paul rosa sven tina uwe vera willi yara zoe")
	lastNames  = strings.Fields("meyer schmidt fischer weber wagner becker hoffmann koch richter klein wolf neumann braun zimmer krause hartmann")
	freemail   = strings.Fields("gmail.com web.de gmx.de outlook.com yahoo.com")
	subjects   = []string{"Angebot", "Follow-up", "Kickoff", "Vertrag", "Rechnung", "Projektplan", "Termin", "Feedback", "Proposal", "Status update", "Pricing question", "Next steps", "Workshop", "Renewal"}
	words      = strings.Fields("we discussed the proposal and agreed to move forward with the next phase of the project please find attached the updated plan the budget looks fine but legal needs another review before we can sign let us meet next week to go through the open points thanks for the quick reply")
)

func messageID(box, n int) string { return fmt.Sprintf("%x%06x", box+1, n) }

func (s *server) rng(box, n int) *rand.Rand {
	//nolint:gosec // G404: seeded on purpose, so a mailbox is the same on every run
	return rand.New(rand.NewPCG(s.boxes[box].seed, uint64(n)))
}

func (s *server) correspondent(r *rand.Rand) (name, addr string) {
	if r.IntN(10) == 0 {
		i := r.IntN(freemailUsers)
		local := firstNames[i%len(firstNames)] + "." + lastNames[(i/3)%len(lastNames)]
		return displayName(local), fmt.Sprintf("%s%d@%s", local, i, freemail[i%len(freemail)])
	}
	x := r.Float64()
	co := min(companies-1, int(companies*x*x*x))
	p := r.IntN(addressesPerCompany)
	first, last := firstNames[(co*7+p)%len(firstNames)], lastNames[(co+p*5)%len(lastNames)]
	return displayName(first + " " + last), fmt.Sprintf("%s.%s@company%03d.example", first, last, co)
}

func displayName(s string) string {
	parts := strings.FieldsFunc(s, func(c rune) bool { return c == ' ' || c == '.' })
	for i, p := range parts {
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// build derives message n of a mailbox from the mailbox seed alone, so the
// server keeps no state and serves any id in any order.
func (s *server) build(box, n int) message {
	owner := s.boxes[box].owner
	r := s.rng(box, n)
	date := s.now.Add(-time.Duration(n)*3*time.Hour - time.Duration(r.IntN(3600))*time.Second)
	root := n
	if n > 0 && r.IntN(3) > 0 {
		root = n + 1 + r.IntN(6)
	}
	thread := s.rng(box, root+1_000_000)
	peerName, peer := s.correspondent(thread)
	subject := subjects[thread.IntN(len(subjects))] + " " + strconv.Itoa(root%97)
	from, to, labels := fmt.Sprintf("%s <%s>", peerName, peer), owner, []string{"INBOX"}
	var extra [][2]string
	switch kind := r.IntN(100); {
	case kind < 40:
		from, to, labels = owner, fmt.Sprintf("%s <%s>", peerName, peer), []string{"SENT"}
	case kind < 52:
		list := r.IntN(newsletters)
		from = fmt.Sprintf("News %d <news@newsletter%02d.example>", list, list)
		subject, root, labels = "Weekly digest "+strconv.Itoa(n), n, []string{"INBOX", "CATEGORY_PROMOTIONS"}
		extra = [][2]string{{"List-Unsubscribe", fmt.Sprintf("<mailto:unsub@newsletter%02d.example>", list)}, {"Precedence", "bulk"}}
	case kind < 58:
		from = s.boxes[(box+1)%len(s.boxes)].owner
	}
	if root != n {
		subject = "Re: " + subject
	}
	headers := [][2]string{{"From", from}, {"To", to}, {"Subject", subject}, {"Date", date.Format(time.RFC1123Z)}, {"Message-ID", fmt.Sprintf("<m%d.%d@mx.demo>", box, n)}}
	if r.IntN(4) == 0 {
		ccName, cc := s.correspondent(r)
		headers = append(headers, [2]string{"Cc", fmt.Sprintf("%s <%s>", ccName, cc)})
	}
	if root != n {
		ref := fmt.Sprintf("<m%d.%d@mx.demo>", box, root)
		headers = append(headers, [2]string{"In-Reply-To", ref}, [2]string{"References", ref})
	}
	headers = append(append(headers, extra...), [2]string{"MIME-Version", "1.0"})
	return message{id: messageID(box, n), labels: labels, date: date, headers: headers, raw: s.body(r, headers, peerName)}
}

func (s *server) body(r *rand.Rand, headers [][2]string, signer string) string {
	var b strings.Builder
	for _, h := range headers {
		b.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	text := prose(r, 40+r.IntN(400), signer)
	size, attached := s.attachmentSize(r)
	if !attached {
		b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n" + text)
		return b.String()
	}
	b.WriteString("Content-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n--b1\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(text + "\r\n--b1\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"plan.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	encoded := base64.StdEncoding.EncodeToString(noise(r.Uint64(), size))
	for len(encoded) > 76 {
		b.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	b.WriteString(encoded + "\r\n--b1--\r\n")
	return b.String()
}

// attachmentSize picks whether a message carries a file and how large. The
// default keeps files small; -big gives 15 in 100 messages a file, a twentieth
// of them between 5 and 15 MiB.
func (s *server) attachmentSize(r *rand.Rand) (int, bool) {
	if !s.big {
		return 20<<10 + r.IntN(380<<10), r.IntN(10) == 0
	}
	if r.IntN(100) >= 15 {
		return 0, false
	}
	switch p := r.IntN(100); {
	case p < 70:
		return 50<<10 + r.IntN(450<<10), true
	case p < 95:
		return 512<<10 + r.IntN(4<<20), true
	default:
		return 5<<20 + r.IntN(10<<20), true
	}
}

// noise is incompressible filler, so a stored file costs what a real one does.
func noise(seed uint64, size int) []byte {
	out := make([]byte, size)
	for i := 0; i+8 <= len(out); i += 8 {
		seed = seed*6364136223846793005 + 1442695040888963407
		binary.LittleEndian.PutUint64(out[i:], seed)
	}
	return out
}

func prose(r *rand.Rand, n int, signer string) string {
	var b strings.Builder
	b.WriteString("Hi,\r\n\r\n")
	for i := range n {
		b.WriteString(words[r.IntN(len(words))])
		if i%14 == 13 {
			b.WriteString(".\r\n")
		} else {
			b.WriteByte(' ')
		}
	}
	b.WriteString("\r\n\r\nBest regards\r\n" + signer + "\r\nPhone +49 30 " + strconv.Itoa(1000000+r.IntN(8999999)) + "\r\n")
	return b.String()
}
