#!/usr/bin/env bash

cat >$1 <<EOF
package main

//go:generate bash $0 $1

const (
    CommitBranch = "$(git symbolic-ref --short HEAD)"
    CommitRef = "$(git describe --always HEAD)"
    CommitHash = "$(git log -n1 --format="%H")"
    CommitDate = "$(git log -n1 --format="%cI" )"
    CommitDateUnix = $(git log -n1 --date=unix --format="%cd" )

    BuildDate = "$(date +"%Y-%m-%dT%H:%M:%S%:z")"
    BuildDateUnixTS = $(date +%s)
)
EOF