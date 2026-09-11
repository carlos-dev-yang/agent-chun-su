package setupskills

import "embed"

// Assets are setup-only guidance. They are never added to report execution packages.
//
//go:embed connect-services
var Assets embed.FS
