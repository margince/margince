---
layers:
  - vn/vn.md
---

# Vietnamese messaging policy

The layers that `gen-messaging-rules` compiles into `messagingrules_gen.go`, most general first.
Vietnam states one layer, the country layer in `vn/vn.md`.

Edit a layer, then run `go generate` in `extensions/apac`, or `make -C backend gen`. The drift lane
fails a generated file that no longer matches these sources.
