package main

// engineTag pins the engine release these loops run against. The
// workflows need the pi agent and github-pr tracker, which ship in the
// kinged007/sortie fork releases; re-pin here when a fork release adds
// what a workflow needs. The version used in asset and cache names is
// this tag without its leading "v" (see engineVersion).
//
// Neither fork release is installable yet: both publish a single bare
// sortie-linux-amd64 with no checksums.txt and no other platform. The pin
// is the version the workflows are written against, and the asset layout
// the resolver needs is sortie_<version>_<os>_<arch>.tar.gz plus
// checksums.txt, which is what a GoReleaser release from the fork
// produces. See "Fork releases need checksums" in the fork's issues.
const engineTag = "v1.24.0-pi.github-pr.1"

// version is this tool's own version, stamped at build time by GoReleaser
// (-X main.version). It is "dev" for a plain `go build`.
var version = "dev"
