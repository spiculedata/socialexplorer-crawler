package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// SeedFileDir returns the fixed directory that seed files named in a job
// payload (the urlFile field) are resolved against. It can be overridden with
// the CRAWLER_SEED_DIR environment variable; the default is "seeds" relative
// to the working directory (/app/seeds in the crawler container).
func SeedFileDir() string {
	if dir := os.Getenv("CRAWLER_SEED_DIR"); dir != "" {
		return dir
	}
	return "seeds"
}

// ResolveSeedFile resolves a seed-file name supplied in a job payload to a
// path inside SeedFileDir, rejecting anything that points outside it.
//
// Job payloads come from the scheduler, where anyone with job-scheduling
// rights controls the urlFile field. Without this check such a payload could
// make the crawler read an arbitrary local file (stored session credentials,
// a service-account token, /proc/self/environ) and treat its contents as seed
// URLs, which then end up in the logs and in the crawl's page list.
//
// Absolute paths and paths containing ".." are rejected outright rather than
// silently rewritten, so a misconfigured payload fails loudly. The name is
// always treated as relative to the seed directory, regardless of the leading
// path components it carries.
func ResolveSeedFile(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("seed file name is empty")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("seed file %q must be relative to the seed directory %q: absolute paths are not allowed", name, SeedFileDir())
	}
	root := filepath.Clean(SeedFileDir())
	resolved := filepath.Join(root, name)
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("seed file %q escapes the seed directory %q and is not allowed", name, root)
	}
	log.Debug().Str("requested", name).Str("resolved", resolved).Msg("Resolved seed file path")
	return resolved, nil
}
