package main

//go:generate bash ./versionInfo.sh version.go

const (
    CommitBranch = "master"
    CommitRef = "1.0-64-g59bbda7"
    CommitHash = "59bbda7345d125cd76bc42b737d95d2265147aba"
    CommitDate = "2018-02-14T00:23:08+01:00"
    CommitDateUnix = 1518564188

    BuildDate = "2018-02-14T00:24:38+01:00"
    BuildDateUnixTS = 1518564279
)
