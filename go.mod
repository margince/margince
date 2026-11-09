// The repository root as a Go module, so a client's go.mod can fetch what no
// Go package holds: the web app, the Makefiles, scripts and go.work. Go leaves
// every nested module (backend, composition, extensions) out of its zip.
module github.com/margince/margince

go 1.27.3
