package main

// sortieVersion pins the sortie binary this loop revision runs against.
// The workflows need the pi agent and github-pr tracker, which are not in
// any sortie release yet (v1.24.0 lacks both). Until a release ships them,
// install sortie from source (see install.sh) and re-pin here.
const sortieVersion = "1.24.0+pi.github-pr"
