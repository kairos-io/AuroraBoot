/*
Copyright © 2026 The Kairos Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/otiai10/copy"
)

// OverlayCopyOptions returns the copy.Options used when merging an
// `--overlay-iso` directory onto an ISO root tree.
//
// Two behaviours matter for Kubernetes-native callers (kairos-operator's
// `OSArtifact.spec.overlayISOVolume` mounts a Secret/ConfigMap directly as
// the overlay):
//
//   - Symlinks are dereferenced (copy.Deep). A kubelet "atomic writer" mount
//     is made entirely of symlinks into a hidden `..data` directory; copied
//     verbatim, a top-level symlink such as `boot -> ..data/boot` shadows the
//     ISO's real boot/ directory and xorriso later fails building the El
//     Torito boot catalog with an opaque "exit status 5"
//     (https://github.com/kairos-io/kairos/issues/4324).
//   - Kubelet atomic-writer internals are skipped: any *direct child* of the
//     overlay root whose name starts with `..` (the live `..data` symlink and
//     the timestamped `..2026_…` backup directories). The check is scoped to
//     direct children on purpose: when a top-level symlink like
//     `boot -> ..data/boot` is dereferenced, otiai10/copy re-invokes Skip
//     with the *target* path (`<root>/..data/boot`), and that content is
//     exactly what we must keep.
//
// src is the overlay root, used to scope the top-level skip check.
func OverlayCopyOptions(src string) copy.Options {
	root := filepath.Clean(src)
	return copy.Options{
		OnSymlink: func(string) copy.SymlinkAction {
			return copy.Deep
		},
		Skip: func(_ os.FileInfo, srcPath, _ string) (bool, error) {
			if filepath.Dir(filepath.Clean(srcPath)) != root {
				return false, nil
			}
			return strings.HasPrefix(filepath.Base(srcPath), ".."), nil
		},
	}
}

// MaterializeOverlay copies an overlay directory into a fresh temporary
// directory, dereferencing symlinks and dropping Kubernetes atomic-writer
// internals (see OverlayCopyOptions), and returns its path. The caller owns
// the returned directory and should remove it once the ISO build is done.
//
// Use this instead of passing a possibly symlinked overlay directory
// straight to a `dir:` image source: rsync (via elemental's DumpSource)
// preserves symlinks, which breaks the ISO build when the overlay is a
// Kubernetes Secret/ConfigMap volume mount.
func MaterializeOverlay(src string) (string, error) {
	dst, err := os.MkdirTemp("", "auroraboot-overlay")
	if err != nil {
		return "", fmt.Errorf("creating temp dir for overlay: %w", err)
	}
	if err := copy.Copy(src, dst, OverlayCopyOptions(src)); err != nil {
		os.RemoveAll(dst)
		return "", fmt.Errorf("materializing overlay %s: %w", src, err)
	}
	return dst, nil
}
