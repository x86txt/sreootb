package fs

import (
	"embed"
	"io/fs"
	"net/http"
)

// GetFrontendFS returns a filesystem for the embedded Next.js app,
// stripping the top-level directory prefix.
func GetFrontendFS(efs embed.FS, prefix string) (http.FileSystem, error) {
	fsys, err := fs.Sub(efs, prefix)
	if err != nil {
		return nil, err
	}
	return http.FS(fsys), nil
}
