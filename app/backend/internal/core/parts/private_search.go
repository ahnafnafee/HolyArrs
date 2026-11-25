package parts

import "os"

// PrivateReleaseSearchEnabled reports whether the install opted into private release search.
func PrivateReleaseSearchEnabled() bool {
	return os.Getenv("HOLYARRS_PRIVATE_RELEASE_SEARCH") == "1"
}
