// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

//go:build !integration

package gates

// The CI image builds from pins in this tree and from nothing else.
//
// The merge gate's toolchain jobs run in that image, so a byte fetched by name
// can change under a green commit. Each shape that lets one in is held here,
// and each pin must also be one Renovate can move.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const ciImageDockerfile = ".github/ci-image/Dockerfile"

var (
	imageDigest      = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	snapshotArg      = regexp.MustCompile(`(?m)^ARG UBUNTU_SNAPSHOT=\d{8}T\d{6}Z$`)
	aptPin           = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]+=[0-9][^\s\\]*$`)
	playwrightArg    = regexp.MustCompile(`(?m)^ARG PLAYWRIGHT_VERSION=(\S+)$`)
	lockedPlaywright = regexp.MustCompile(`(?m)^  playwright-core@(\d+\.\d+\.\d+):`)
	// Each switch turns off a check a download would otherwise pass:
	// Go's checksum database, or corepack's signature on the pnpm it fetches.
	verificationOff = regexp.MustCompile(`GOSUMDB=off|GONOSUMDB|GONOSUMCHECK|GOINSECURE|GOPRIVATE|GOFLAGS=\S*-insecure|COREPACK_INTEGRITY_KEYS`)
)

// The rewrite a stage needs before apt: the image's live sources removed, and
// the snapshot named at the one timestamp ARG Renovate moves.
var snapshotSources = []string{
	"rm /etc/apt/sources.list.d/ubuntu.sources",
	"URIs: https://snapshot.ubuntu.com/ubuntu/${UBUNTU_SNAPSHOT}",
	"/etc/apt/sources.list.d/snapshot.sources",
}

// liveArchives are the hosts that serve today's packages rather than a pinned
// timestamp's.
var liveArchives = []string{"archive.ubuntu.com", "security.ubuntu.com", "ports.ubuntu.com", "deb.debian.org", "security.debian.org"}

// ciImageCensus counts what a reading found, so a parser that stopped
// recognising this file reports as short rather than as clean.
type ciImageCensus struct {
	images, downloads, snapshotStages int
	packages                          []string
}

func TestTheCIImageBuildsOnlyFromPins(t *testing.T) {
	t.Parallel()
	body := readCIImageDockerfile(t)
	findings, census := ciImageFindings(body)
	for _, finding := range findings {
		t.Errorf("%s: %s", ciImageDockerfile, finding)
	}
	// The floor is the image as built today: Go, Node and Ubuntu, the snapshot
	// stage, the browser download, and the packages Chromium alone needs.
	if census.images < 3 || census.snapshotStages < 1 || census.downloads < 1 || len(census.packages) < 30 {
		t.Errorf("%s read short: %d images, %d snapshot stages, %d downloads, %d pinned packages — "+
			"either the image lost a part or this reading stopped recognising it",
			ciImageDockerfile, census.images, census.snapshotStages, census.downloads, len(census.packages))
	}
}

// Each mutation is one way an unpinned byte gets in. The patterns match a pin's
// shape, so a Renovate bump leaves them aimed. A case that changes nothing would
// test nothing, so each must change the file.
func TestTheCIImagePinGateRefusesEachUnpinnedShape(t *testing.T) {
	t.Parallel()
	body := readCIImageDockerfile(t)
	mutations := []struct{ name, pattern, replacement string }{
		{"a package with no version", `(?m)^(\s+make)=\S+`, "$1"},
		{"a base image with a tag and no digest", `(FROM node:\S+)@sha256:[0-9a-f]{64}`, "$1"},
		{"a COPY source with a tag and no digest", `COPY --from=go `, "COPY --from=golang:trixie "},
		{"a download with no checksum", `sha256sum -c -`, "true"},
		{"an apt source outside the snapshot", `https://snapshot\.ubuntu\.com/ubuntu/\$\{UBUNTU_SNAPSHOT\}`, "http://archive.ubuntu.com/ubuntu"},
		{"the live sources left in place", `rm /etc/apt/sources\.list\.d/ubuntu\.sources \\\n && `, ""},
		{"an update that lets a missing index pass", `apt-get update --error-on=any`, "apt-get update"},
		{"Go's checksum database turned off", `ENV GOPATH=/go GOTOOLCHAIN=local`, "ENV GOPATH=/go GOTOOLCHAIN=local GOSUMDB=off"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			t.Parallel()
			pattern := regexp.MustCompile(mutation.pattern)
			if !pattern.MatchString(body) {
				t.Fatalf("the Dockerfile no longer matches %q, so this case tests nothing; aim it at the current shape", mutation.pattern)
			}
			mutated := pattern.ReplaceAllString(body, mutation.replacement)
			if findings, _ := ciImageFindings(mutated); len(findings) == 0 {
				t.Errorf("%s passed the gate", mutation.name)
			}
		})
	}
}

// A flag before the subcommand must not hide the packages behind it.
func TestTheCIImagePinGateReadsAnInstallAfterItsFlags(t *testing.T) {
	t.Parallel()
	const stage = "FROM scratch AS base\n" +
		"RUN rm /etc/apt/sources.list.d/ubuntu.sources && " +
		"echo 'URIs: https://snapshot.ubuntu.com/ubuntu/${UBUNTU_SNAPSHOT}' > /etc/apt/sources.list.d/snapshot.sources\n"
	findings, _ := ciImageFindings(stage + "RUN apt-get -y install --no-install-recommends make\n")
	if !slices.ContainsFunc(findings, func(finding string) bool { return strings.Contains(finding, "installs make") }) {
		t.Errorf("`apt-get -y install make` passed with no version; findings: %v", findings)
	}
}

// Two downloads behind one checksum check leave the second file unchecked. A
// flag before `update` must not hide an update that lets a lost index pass.
func TestTheCIImagePinGateHoldsEachDownloadAndEachUpdate(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"downloads 2 files and checks 1": "RUN curl -o /a https://x/a && echo 'sum  /a' | sha256sum -c - && curl -o /b https://x/b\n",
		"without --error-on=any":         "RUN apt-get -q update\n",
	}
	for want, run := range cases {
		findings, _ := ciImageFindings("FROM scratch AS base\n" + run)
		if !slices.ContainsFunc(findings, func(finding string) bool { return strings.Contains(finding, want) }) {
			t.Errorf("%q passed; want a finding naming %q, got %v", strings.TrimSpace(run), want, findings)
		}
	}
}

// The browser in the image is the build one Playwright version launches. When
// the lockfile moves Playwright and the image does not, every UAT shard fails
// to find its browser, so the two are held together here.
func TestTheCIImageChromiumIsTheLockedPlaywrights(t *testing.T) {
	t.Parallel()
	pinned := playwrightArg.FindStringSubmatch(readCIImageDockerfile(t))
	if pinned == nil {
		t.Fatalf("%s carries no `ARG PLAYWRIGHT_VERSION=`", ciImageDockerfile)
	}
	lock, err := os.ReadFile(filepath.Join(repoRoot, "pnpm-lock.yaml"))
	if err != nil {
		t.Fatalf("reading pnpm-lock.yaml: %v", err)
	}
	locked := map[string]bool{}
	for _, match := range lockedPlaywright.FindAllStringSubmatch(string(lock), -1) {
		locked[match[1]] = true
	}
	if len(locked) != 1 || !locked[pinned[1]] {
		t.Errorf("%s pins Playwright %s and the lockfile resolves playwright-core %v. "+
			"Set PLAYWRIGHT_VERSION, CHROMIUM_REVISION and CHROMIUM_VERSION from that release's browsers.json, "+
			"and CHROMIUM_SHA256 from the headless shell it downloads", ciImageDockerfile, pinned[1], locked)
	}
}

func readCIImageDockerfile(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, ciImageDockerfile))
	if err != nil {
		t.Fatalf("reading the CI image's Dockerfile: %v", err)
	}
	return string(body)
}

// ciImageFindings reads a Dockerfile instruction by instruction and names every
// unpinned input it finds.
func ciImageFindings(body string) ([]string, ciImageCensus) {
	var findings []string
	var census ciImageCensus
	if !snapshotArg.MatchString(body) {
		findings = append(findings, "no `ARG UBUNTU_SNAPSHOT=<YYYYMMDDTHHMMSSZ>`, so the snapshot has no pin Renovate can move")
	}
	stages := map[string]bool{}
	snapshotted := map[string]bool{}
	current := ""
	for _, instruction := range dockerInstructions(body) {
		word, rest, _ := strings.Cut(instruction, " ")
		switch strings.ToUpper(word) {
		case "FROM":
			image, name := fromImageAndStage(rest)
			if !stages[strings.ToLower(image)] && image != "scratch" {
				census.images++
				findings = appendUnpinned(findings, "FROM", image)
			}
			current = strings.ToLower(name)
			stages[current] = true
			snapshotted[current] = snapshotted[strings.ToLower(image)]
		case "COPY", "ADD":
			findings = append(findings, copySourceFindings(instruction, stages, &census)...)
		case "RUN":
			if isSnapshotRewrite(instruction) {
				snapshotted[current] = true
				census.snapshotStages++
			}
			findings = append(findings, runFindings(instruction, snapshotted[current], &census)...)
		}
		if verificationOff.MatchString(instruction) {
			findings = append(findings, "turns a download's verification off: "+instruction)
		}
		for _, host := range liveArchives {
			if strings.Contains(instruction, host) {
				findings = append(findings, "names "+host+", a live archive rather than the snapshot: "+instruction)
			}
		}
	}
	return findings, census
}

// dockerInstructions joins continued lines and drops comments, which is how
// the builder reads the file.
func dockerInstructions(body string) []string {
	var instructions []string
	var pending strings.Builder
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || (trimmed == "" && pending.Len() == 0) {
			continue
		}
		continued := strings.HasSuffix(trimmed, "\\")
		pending.WriteString(strings.TrimSuffix(trimmed, "\\"))
		pending.WriteString(" ")
		if !continued {
			instructions = append(instructions, strings.Join(strings.Fields(pending.String()), " "))
			pending.Reset()
		}
	}
	return instructions
}

// fromImageAndStage reads `FROM [--flag…] <image> [AS <name>]`.
func fromImageAndStage(rest string) (image, stage string) {
	var words []string
	for _, word := range strings.Fields(rest) {
		if !strings.HasPrefix(word, "--") {
			words = append(words, word)
		}
	}
	if len(words) == 0 {
		return "", ""
	}
	if len(words) >= 3 && strings.EqualFold(words[1], "AS") {
		return words[0], words[2]
	}
	return words[0], ""
}

func appendUnpinned(findings []string, what, image string) []string {
	if imageDigest.MatchString(image) {
		return findings
	}
	return append(findings, fmt.Sprintf("%s %s has no @sha256 digest", what, image))
}

// copySourceFindings holds a `COPY --from` image to the digest rule of a
// `FROM`, unless it names an earlier stage. An ADD of a URL needs a checksum.
func copySourceFindings(instruction string, stages map[string]bool, census *ciImageCensus) []string {
	var findings []string
	for _, word := range strings.Fields(instruction) {
		source, isFrom := strings.CutPrefix(word, "--from=")
		if isFrom && !stages[strings.ToLower(source)] {
			census.images++
			findings = appendUnpinned(findings, "COPY --from=", source)
		}
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			census.downloads++
			if !strings.Contains(instruction, "--checksum=sha256:") {
				findings = append(findings, "downloads with no checksum: "+instruction)
			}
		}
	}
	return findings
}

func isSnapshotRewrite(instruction string) bool {
	for _, part := range snapshotSources {
		if !strings.Contains(instruction, part) {
			return false
		}
	}
	return true
}

// runFindings holds one RUN's apt calls to the snapshot and to exact versions,
// and its downloads to a checksum it checks.
func runFindings(instruction string, snapshotted bool, census *ciImageCensus) []string {
	var findings []string
	fetches := 0
	for _, command := range shellCommands(instruction) {
		words := strings.Fields(command)
		for i, word := range words {
			if (word == "apt-get" || word == "apt") && !snapshotted {
				findings = append(findings, "runs apt in a stage whose sources are not the snapshot: "+command)
			}
			if word == "apt-get" || word == "apt" {
				findings = append(findings, installedPackages(words[i+1:], census)...)
				findings = append(findings, silentUpdate(words[i+1:])...)
			}
			if word == "curl" || word == "wget" {
				census.downloads++
				fetches++
			}
		}
	}
	if checks := strings.Count(instruction, "sha256sum -c"); fetches > checks {
		findings = append(findings, fmt.Sprintf("downloads %d files and checks %d checksums: %s", fetches, checks, instruction))
	}
	return findings
}

// shellCommands splits a RUN at its `&&`, `;` and `|` joins, so one install's
// argument list ends where the next command starts.
func shellCommands(instruction string) []string {
	return regexp.MustCompile(`&&|\|\||;|\|`).Split(instruction, -1)
}

// installedPackages finds the subcommand past any leading flags, so
// `apt-get -y install` is read like `apt-get install -y`.
func installedPackages(args []string, census *ciImageCensus) []string {
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if arg != "install" {
			return nil
		}
		return unversionedPackages(args[i+1:], census)
	}
	return nil
}

// silentUpdate refuses an `apt-get update` that may end with an index missing:
// apt only warns, and every pin after it then reads as not found.
func silentUpdate(args []string) []string {
	flags := 0
	for flags < len(args) && strings.HasPrefix(args[flags], "-") {
		flags++
	}
	if flags == len(args) || args[flags] != "update" || slices.Contains(args, "--error-on=any") {
		return nil
	}
	return []string{"runs apt-get update without --error-on=any, so a missing index passes as a warning"}
}

func unversionedPackages(args []string, census *ciImageCensus) []string {
	var findings []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if !aptPin.MatchString(arg) {
			findings = append(findings, "installs "+arg+" with no exact version")
			continue
		}
		census.packages = append(census.packages, arg)
	}
	return findings
}

// Renovate moves a pin only if its own pattern finds it, so the patterns in
// renovate.json run over the Dockerfile itself. Every package the build
// installs must be one the package manager reads. The snapshot manager must
// read the one timestamp.
func TestRenovateReachesEveryCIImagePin(t *testing.T) {
	t.Parallel()
	body := readCIImageDockerfile(t)
	_, census := ciImageFindings(body)
	if len(census.packages) == 0 {
		t.Fatalf("%s: the package census is empty, so this check would assert nothing", ciImageDockerfile)
	}
	found := map[string]int{}
	for _, manager := range readRenovateConfig(t).CustomManagers {
		if !anyMatches(t, manager.ManagerFilePatterns, ciImageDockerfile) {
			continue
		}
		for _, matchString := range manager.MatchStrings {
			for _, hit := range goPinNamed(t, matchString).FindAllStringSubmatch(body, -1) {
				found[manager.DatasourceTemplate]++
				if manager.DatasourceTemplate == "custom.ubuntu-package" {
					found[hit[1]+"="+hit[2]]++
				}
			}
		}
	}
	for _, pin := range census.packages {
		if found[pin] != 1 {
			t.Errorf("%s installs %s, and renovate.json's package pattern finds it %d times; "+
				"a pin it does not find once is a pin nothing moves", ciImageDockerfile, pin, found[pin])
		}
	}
	if found["custom.ubuntu-package"] != len(census.packages) {
		t.Errorf("renovate.json's package pattern finds %d pins in %s and the build installs %d; "+
			"a hit that is no package would be offered Ubuntu versions it cannot take",
			found["custom.ubuntu-package"], ciImageDockerfile, len(census.packages))
	}
	if found["custom.ubuntu-snapshot"] != 1 {
		t.Errorf("renovate.json's snapshot pattern finds %d timestamps in %s, want exactly the one ARG",
			found["custom.ubuntu-snapshot"], ciImageDockerfile)
	}
}
