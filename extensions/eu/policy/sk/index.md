---
layers:
  - regional/eu.md
  - sk/sk.md
---

# Slovak messaging policy

The layers that `gen-messaging-rules` compiles into `messagingrules_gen.go`, most general first:
the EU layer every member state shares, then the country layer in `sk/sk.md`.

Edit a layer, then run `go generate` in `extensions/eu`, or `make -C backend gen`. The drift lane
fails a generated file that no longer matches these sources.
