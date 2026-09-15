package main

// sortieVersion pins the sortie binary this loop revision runs against.
// The workflows need the pi agent and github-pr tracker, which ship in
// the kinged007/sortie fork releases (see install.sh); re-pin here when
// the fork cuts a new release the workflows are built against.
const sortieVersion = "1.24.0+pi.github-pr.1"
