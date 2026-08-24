// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

// A site writes a robots.txt group naming the bot it saw in its logs. What it
// saw is the User-Agent this crawler SENDS; what decides whether that group
// applies is the token the matcher READS. If those two ever stop being the same
// string, a Disallow the site wrote for us stops matching, and the crawler goes
// on reading pages the site refused — obeying a policy while ignoring the one
// written for it.
//
// Nothing else here can see that. Every other robots test writes its group from
// `robotsAgentProduct`, the same constant the matcher reads, so the group
// matches by construction whatever the header says. This test builds the group
// from the ADVERTISED header instead, which is the only version of the question
// a site operator can ask.

import (
	"strings"
	"testing"
	"time"
)

func TestARobotsGroupNamingWhatWeAdvertiseAppliesToUs(t *testing.T) {
	t.Parallel()

	// The product half of the header, the way a site operator reads it out of
	// their log: everything before the version separator.
	advertised, _, _ := strings.Cut(UserAgent, "/")
	if advertised == "" {
		t.Fatal("the crawler advertises no product token, so no site can write a rule about it")
	}

	robots := "User-agent: *\nCrawl-delay: 1\n\nUser-agent: " + advertised + "\nCrawl-delay: 4\n"
	if got := parseRobots(robots).crawlDelay; got != 4*time.Second {
		t.Errorf("a group naming %q got crawlDelay %v, want 4s — a site's rule for the name we "+
			"advertise does not reach the matcher, which reads %q",
			advertised, got, robotsAgentProduct)
	}

	// And a Disallow written for that name is obeyed, which is the directive
	// that actually costs a site something when it is missed.
	blocked := parseRobots("User-agent: " + advertised + "\nDisallow: /private\n")
	if blocked.allows("/private/report.html") {
		t.Errorf("a Disallow written for %q does not refuse the path it names", advertised)
	}

	// The whole header, which is what an operator copies out of their log. A
	// matcher tightened to exact equality would stop honouring the form people
	// actually write, and nothing else here would notice.
	fromTheLog := parseRobots("User-agent: " + UserAgent + "\nDisallow: /private\n")
	if fromTheLog.allows("/private/report.html") {
		t.Errorf("a Disallow written for the full header %q is not honoured", UserAgent)
	}
}

// A rule written for a DIFFERENT bot must not reach this one.
//
// The mirror of the test above, and the direction a substring match gets wrong:
// a named group outranks the wildcard, so a group for some other bot whose name
// merely contains ours would divert this crawler off a site-wide Disallow and it
// would crawl a site that refused it. Matching too much fails open; matching too
// little only costs a rate limit.
func TestARobotsGroupForAnotherBotDoesNotReachUs(t *testing.T) {
	t.Parallel()

	for _, other := range []string{
		robotsAgentProduct + "-legacy", // a superstring: what Contains admitted
		"not-" + robotsAgentProduct,    // ours as a suffix
		"margince",                     // a proper substring of ours
		"margince-geocode",             // this product's OTHER agent
	} {
		policy := parseRobots("User-agent: *\nDisallow: /\n\nUser-agent: " + other + "\nAllow: /\n")
		if policy.allows("/anything") {
			t.Errorf("a group naming %q diverted this crawler off the wildcard Disallow — "+
				"a rule written for another bot let us past a site-wide refusal", other)
		}
	}
}
