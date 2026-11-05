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
	"strings"
	"testing"
)

const ciImageDockerfile = ".github/ci-image/Dockerfile"

var (
	imageDigest      = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	snapshotArg      = regexp.MustCompile(`(?m)^ARG DEBIAN_SNAPSHOT=\d{8}T\d{6}Z$`)
	aptPin           = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]+=[0-9][^\s\\]*$`)
	playwrightArg    = regexp.MustCompile(`(?m)^ARG PLAYWRIGHT_VERSION=(\S+)$`)
	lockedPlaywright = regexp.MustCompile(`(?m)^  playwright-core@(\d+\.\d+\.\d+):`)
	// Each switch turns off a check a download would otherwise pass:
	// Go's checksum database, or corepack's signature on the pnpm it fetches.
	verificationOff = regexp.MustCompile(`GOSUMDB=off|GONOSUMDB|GONOSUMCHECK|GOINSECURE|GOPRIVATE|GOFLAGS=\S*-insecure|COREPACK_INTEGRITY_KEYS`)
)

// The snapshot both URIs must name, at the one timestamp ARG Renovate moves.
var snapshotSources = []string{
	"http://snapshot-cloudflare.debian.org/archive/debian/${DEBIAN_SNAPSHOT}",
	"http://snapshot-cloudflare.debian.org/archive/debian-security/${DEBIAN_SNAPSHOT}",
	`Acquire::Check-Valid-Until "false";`,
	"/etc/apt/sources.list.d/debian.sources",
}

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
	// The floor is the image as built today: Go, Node and Debian, the snapshot
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
		{"an apt source outside the snapshot", `http://snapshot-cloudflare\.debian\.org/archive/debian/\$\{DEBIAN_SNAPSHOT\}`, "http://deb.debian.org/debian"},
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
		findings = append(findings, "no `ARG DEBIAN_SNAPSHOT=<YYYYMMDDTHHMMSSZ>`, so the snapshot has no pin Renovate can move")
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
		if strings.Contains(instruction, "deb.debian.org") || strings.Contains(instruction, "security.debian.org") {
			findings = append(findings, "names a live Debian mirror rather than the snapshot: "+instruction)
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
	for _, command := range shellCommands(instruction) {
		words := strings.Fields(command)
		for i, word := range words {
			if (word == "apt-get" || word == "apt") && !snapshotted {
				findings = append(findings, "runs apt in a stage whose sources are not the snapshot: "+command)
			}
			if (word == "apt-get" || word == "apt") && i+1 < len(words) && words[i+1] == "install" {
				findings = append(findings, unversionedPackages(words[i+2:], census)...)
			}
			if word == "curl" || word == "wget" {
				census.downloads++
				if !strings.Contains(instruction, "sha256sum -c") {
					findings = append(findings, "downloads with no checksum: "+command)
				}
			}
		}
	}
	return findings
}

// shellCommands splits a RUN at its `&&`, `;` and `|` joins, so one install's
// argument list ends where the next command starts.
func shellCommands(instruction string) []string {
	return regexp.MustCompile(`&&|\|\||;|\|`).Split(instruction, -1)
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
	found := map[string]int{}
	for _, manager := range readRenovateConfig(t).CustomManagers {
		if !anyMatches(t, manager.ManagerFilePatterns, ciImageDockerfile) {
			continue
		}
		for _, matchString := range manager.MatchStrings {
			for _, hit := range goPinNamed(t, matchString).FindAllStringSubmatch(body, -1) {
				found[manager.DatasourceTemplate]++
				if manager.DatasourceTemplate == "custom.debian-package" {
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
	if found["custom.debian-package"] != len(census.packages) {
		t.Errorf("renovate.json's package pattern finds %d pins in %s and the build installs %d; "+
			"a hit that is no package would be offered Debian versions it cannot take",
			found["custom.debian-package"], ciImageDockerfile, len(census.packages))
	}
	if found["custom.debian-snapshot"] != 1 {
		t.Errorf("renovate.json's snapshot pattern finds %d timestamps in %s, want exactly the one ARG",
			found["custom.debian-snapshot"], ciImageDockerfile)
	}
}
