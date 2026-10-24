package tooling

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var (
	sourceExtensions = []string{".cpp", ".cc", ".cxx", ".c"}
	headerExtensions = []string{".hpp", ".hh", ".hxx", ".h"}
)

// IsSourceFile returns true if the extension is a recognized C/C++ source extension.
func IsSourceFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, s := range sourceExtensions {
		if ext == s {
			return true
		}
	}
	return false
}

// IsHeaderFile returns true if the extension is a recognized C/C++ header extension.
func IsHeaderFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, h := range headerExtensions {
		if ext == h {
			return true
		}
	}
	return false
}

// FindAlternate locates the complementary C/C++ header or source file for the given path.
// It checks:
// 1. Same directory with complementary extensions.
// 2. Transformed paths between src/ and include/ (or source/ and include/).
// 3. Workspace tree search for matching base name.
// If an existing file is found, it returns the existing path.
// Otherwise, it returns a suggested default counterpart path.
func FindAlternate(currentPath string, rootDir string) (string, error) {
	if currentPath == "" {
		return "", errors.New("empty path")
	}

	ext := strings.ToLower(filepath.Ext(currentPath))
	baseWithoutExt := strings.TrimSuffix(filepath.Base(currentPath), ext)
	dir := filepath.Dir(currentPath)

	var targetExts []string
	var defaultTargetExt string

	if IsSourceFile(currentPath) {
		targetExts = headerExtensions
		if ext == ".c" {
			defaultTargetExt = ".h"
		} else {
			defaultTargetExt = ".hpp"
		}
	} else if IsHeaderFile(currentPath) {
		targetExts = sourceExtensions
		if ext == ".h" {
			defaultTargetExt = ".c"
		} else {
			defaultTargetExt = ".cpp"
		}
	} else {
		return "", errors.New("not a recognized C/C++ source or header file")
	}

	// 1. Check same directory
	for _, tExt := range targetExts {
		candidate := filepath.Join(dir, baseWithoutExt+tExt)
		if fileExists(candidate) {
			return candidate, nil
		}
	}

	// 2. Check src <-> include directory transformations
	var counterpartDirs []string
	cleanPath := filepath.Clean(currentPath)

	if strings.Contains(cleanPath, "/src/") || strings.HasPrefix(cleanPath, "src/") {
		counterpartDirs = append(counterpartDirs,
			strings.Replace(cleanPath, "/src/", "/include/", 1),
			strings.Replace(cleanPath, "src/", "include/", 1),
		)
	} else if strings.Contains(cleanPath, "/include/") || strings.HasPrefix(cleanPath, "include/") {
		counterpartDirs = append(counterpartDirs,
			strings.Replace(cleanPath, "/include/", "/src/", 1),
			strings.Replace(cleanPath, "include/", "src/", 1),
		)
	} else if strings.Contains(cleanPath, "/source/") || strings.HasPrefix(cleanPath, "source/") {
		counterpartDirs = append(counterpartDirs,
			strings.Replace(cleanPath, "/source/", "/include/", 1),
			strings.Replace(cleanPath, "source/", "include/", 1),
		)
	}

	for _, cDir := range counterpartDirs {
		cBaseDir := filepath.Dir(cDir)
		for _, tExt := range targetExts {
			candidate := filepath.Join(cBaseDir, baseWithoutExt+tExt)
			if fileExists(candidate) {
				return candidate, nil
			}
		}
	}

	// 3. Search under rootDir if provided
	if rootDir != "" {
		var found string
		_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d == nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "build" || name == "target" || name == "node_modules" || name == ".ccls-cache" {
					return filepath.SkipDir
				}
				return nil
			}

			cExt := strings.ToLower(filepath.Ext(path))
			cBase := strings.TrimSuffix(filepath.Base(path), cExt)
			if cBase == baseWithoutExt {
				for _, tExt := range targetExts {
					if cExt == tExt {
						found = path
						return errors.New("found")
					}
				}
			}
			return nil
		})
		if found != "" {
			return found, nil
		}
	}

	// 4. Default fallback: suggested path
	if len(counterpartDirs) > 0 {
		return filepath.Join(filepath.Dir(counterpartDirs[0]), baseWithoutExt+defaultTargetExt), nil
	}
	return filepath.Join(dir, baseWithoutExt+defaultTargetExt), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
