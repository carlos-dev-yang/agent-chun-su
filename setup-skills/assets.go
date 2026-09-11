package setupskills

import "embed"

// Assets guide setup and conversation. They are never added to report execution packages.
//
//go:embed connect-services converse
var Assets embed.FS
