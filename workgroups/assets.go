package workgroups

import "embed"

//go:embed mail-review/* jira-report/* code-review/*
var Assets embed.FS
