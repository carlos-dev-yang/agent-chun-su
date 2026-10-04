package workgroups

import "embed"

//go:embed mail-review/* jira-report/* code-review/* team-ops/*
var Assets embed.FS
