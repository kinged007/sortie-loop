package main

// sortieVersion pins the sortie binary this loop revision runs against.
// The workflows need the pi agent and github-pr tracker, which are not in
// (v1.24.0 lacks all three; the required-label fix came after
// 2f6d0bd4). Until a release ships them, install sortie from source
// (see install.sh) and re-pin here.
const sortieVersion = "1.24.0+pi.github-pr.1"
