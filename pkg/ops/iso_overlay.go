package ops

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func materializeISOOverlay(source string) (string, func(), error) {
	root, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", nil, fmt.Errorf("resolve ISO overlay root: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", nil, fmt.Errorf("make ISO overlay root absolute: %w", err)
	}

	target, err := os.MkdirTemp("", "auroraboot-iso-overlay-")
	if err != nil {
		return "", nil, fmt.Errorf("create ISO overlay staging directory: %w", err)
	}
	cleanup := func() {
		_ = filepath.WalkDir(target, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
		_ = os.RemoveAll(target)
	}

	activeDirs := map[string]bool{}
	if err := copyISOOverlayEntry(root, root, target, activeDirs); err != nil {
		cleanup()
		return "", nil, err
	}
	return target, cleanup, nil
}

func copyISOOverlayEntry(root, source, target string, activeDirs map[string]bool) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect ISO overlay path %q: %w", source, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(source)
		if err != nil {
			return fmt.Errorf("read ISO overlay symlink %q: %w", source, err)
		}
		resolved, err := filepath.EvalSymlinks(source)
		if err != nil {
			return fmt.Errorf("resolve ISO overlay symlink %q: %w", source, err)
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return fmt.Errorf("make resolved ISO overlay path absolute: %w", err)
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil {
			return fmt.Errorf("check ISO overlay symlink %q: %w", source, err)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("ISO overlay symlink %q escapes overlay root", source)
		}
		metadata := atomicWriterMetadata(filepath.Dir(source), root)
		if metadata["..data"] && isAtomicWriterLink(linkTarget) {
			return copyISOOverlayEntry(root, resolved, target, activeDirs)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create ISO overlay symlink parent: %w", err)
		}
		if err := os.Symlink(linkTarget, target); err != nil {
			return fmt.Errorf("copy ISO overlay symlink %q: %w", source, err)
		}
		return nil
	}

	if info.IsDir() {
		resolved, err := filepath.EvalSymlinks(source)
		if err != nil {
			return fmt.Errorf("resolve ISO overlay directory %q: %w", source, err)
		}
		if activeDirs[resolved] {
			return fmt.Errorf("ISO overlay symlink cycle at %q", source)
		}
		activeDirs[resolved] = true
		defer delete(activeDirs, resolved)

		if err := os.MkdirAll(target, info.Mode().Perm()|0o700); err != nil {
			return fmt.Errorf("create ISO overlay directory %q: %w", target, err)
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return fmt.Errorf("read ISO overlay directory %q: %w", source, err)
		}
		metadata := atomicWriterMetadata(source, root)
		for _, entry := range entries {
			if metadata[entry.Name()] {
				continue
			}
			if err := copyISOOverlayEntry(root, filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name()), activeDirs); err != nil {
				return err
			}
		}
		if err := os.Chmod(target, info.Mode().Perm()); err != nil {
			return fmt.Errorf("set ISO overlay directory mode %q: %w", target, err)
		}
		return nil
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported ISO overlay file type at %q", source)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create ISO overlay parent directory: %w", err)
	}
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open ISO overlay file %q: %w", source, err)
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create materialized ISO overlay file %q: %w", target, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy ISO overlay file %q: %w", source, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close materialized ISO overlay file %q: %w", target, err)
	}
	if err := os.Chmod(target, info.Mode().Perm()); err != nil {
		return fmt.Errorf("set materialized ISO overlay file mode %q: %w", target, err)
	}
	return nil
}

func isAtomicWriterLink(target string) bool {
	if filepath.IsAbs(target) {
		return false
	}
	clean := filepath.Clean(target)
	return clean == "..data" || strings.HasPrefix(clean, "..data"+string(filepath.Separator))
}

func atomicWriterMetadata(dir, root string) map[string]bool {
	metadata := map[string]bool{}
	dataLink := filepath.Join(dir, "..data")
	info, err := os.Lstat(dataLink)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return metadata
	}
	resolved, err := filepath.EvalSymlinks(dataLink)
	if err != nil {
		return metadata
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return metadata
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return metadata
	}
	if filepath.Dir(resolved) != dir {
		return metadata
	}
	metadata["..data"] = true
	metadata[filepath.Base(resolved)] = true
	return metadata
}
