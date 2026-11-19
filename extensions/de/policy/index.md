---
layers:
  - de.md
---

# German messaging policy

The layers that `gen-messaging-rules` compiles into `messagingrules_gen.go`, most general first.
Germany states one layer, the country layer in `de.md`.

Edit a layer, then run `go generate` in `extensions/de`, or `make -C backend gen`. The drift lane
fails a generated file that no longer matches these sources.
