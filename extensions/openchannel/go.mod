module github.com/margince/margince/extensions/openchannel

go 1.26.6

// The backend this unit compiles against is the one IN THIS TREE, not a published
// snapshot of it. pkg/extension has no module version — it resolves through the
// generated build/composition/go.work — and `go mod tidy` ignores workspace `use`
// directives, so without this it goes to the network and pins a pseudo-version of
// whatever commit happened to be pushed. That is worse than failing: the unit then
// compiles against a different backend than the one it ships with, silently.
//
// It is also what makes a dependency bump landable unattended. Renovate's artifact
// update shells out to `go mod tidy`; with no way to resolve the backend locally it
// rewrote go.mod, left go.sum on the old version, and reported an artifact failure
// nobody could fix without hand-writing sums (#2003).
replace github.com/margince/margince/backend => ../../backend

require github.com/margince/margince/backend v0.0.0-00010101000000-000000000000
