# Embedded icon sets

Run `go generate` in this directory to refresh the pinned Font Awesome Free, Lucide, and Tabler Outline sets. The generator downloads release archives, verifies their SHA-256 checksums, and writes each library's `data.txt`, `LICENSE.txt`, and `provenance.json`.

`data.txt` is a deterministic gzip-compressed JSON bundle encoded as base64 text. The parent package embeds the three text files with `go:embed`; rendering never fetches icons from the network.
