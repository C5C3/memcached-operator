package v1alpha1

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestDocumentationLinks validates that all internal links in documentation files
// under docs/ resolve to existing files or directories.
func TestDocumentationLinks(t *testing.T) {
	// Get the project root directory (../../ from this test file)
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get current file path")
	}
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	docsDir := filepath.Join(projectRoot, "docs")

	// Verify docs directory exists
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		t.Fatalf("docs directory does not exist: %s", docsDir)
	}

	// Find all markdown files
	markdownFiles, err := findMarkdownFiles(docsDir)
	if err != nil {
		t.Fatalf("failed to find markdown files: %v", err)
	}

	if len(markdownFiles) == 0 {
		t.Fatal("no markdown files found in docs/ directory")
	}

	// Extract and validate links from all markdown files
	totalLinks := 0
	linkPattern := regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)

	for _, mdFile := range markdownFiles {
		content, err := os.ReadFile(mdFile)
		if err != nil {
			t.Errorf("failed to read %s: %v", mdFile, err)
			continue
		}

		// Find all links in the file
		matches := linkPattern.FindAllStringSubmatch(string(content), -1)
		if len(matches) == 0 {
			continue
		}

		// Get relative path for better test names
		relPath, _ := filepath.Rel(docsDir, mdFile)

		for _, match := range matches {
			linkText := match[1]
			linkTarget := match[2]

			// Skip external URLs
			if strings.HasPrefix(linkTarget, "http://") || strings.HasPrefix(linkTarget, "https://") {
				continue
			}

			// Skip bare anchors (same-file anchors)
			if strings.HasPrefix(linkTarget, "#") {
				continue
			}

			totalLinks++

			// Strip anchor suffix if present
			targetPath := linkTarget
			if idx := strings.Index(targetPath, "#"); idx != -1 {
				targetPath = targetPath[:idx]
			}

			// Skip if it's just an anchor with no path
			if targetPath == "" {
				continue
			}

			// Resolve relative to the containing file's directory
			containingDir := filepath.Dir(mdFile)
			resolvedPath := filepath.Join(containingDir, targetPath)

			// Clean the path to normalize it
			resolvedPath = filepath.Clean(resolvedPath)

			// Create a descriptive test name
			testName := relPath + "/" + targetPath

			t.Run(testName, func(t *testing.T) {
				// Check if the target exists (as file or directory)
				info, err := os.Stat(resolvedPath)
				if os.IsNotExist(err) {
					t.Errorf("broken link in %s: link text=%q, target=%q, resolved path does not exist: %s",
						relPath, linkText, linkTarget, resolvedPath)
					return
				}
				if err != nil {
					t.Errorf("error checking link in %s: link text=%q, target=%q, error: %v",
						relPath, linkText, linkTarget, err)
					return
				}

				// Additional validation: if link ends with .md, ensure it's a file not a directory
				if strings.HasSuffix(targetPath, ".md") && info.IsDir() {
					t.Errorf("invalid link in %s: link text=%q, target=%q points to directory but has .md extension",
						relPath, linkText, linkTarget)
				}

				// If link ends with /, ensure it's a directory not a file
				if strings.HasSuffix(targetPath, "/") && !info.IsDir() {
					t.Errorf("invalid link in %s: link text=%q, target=%q ends with / but points to file",
						relPath, linkText, linkTarget)
				}
			})
		}
	}

	// Sanity check: ensure we found at least some links
	if totalLinks == 0 {
		t.Error("no internal links found across all documentation files - this is unexpected")
	}
}

// findMarkdownFiles recursively finds all .md files in the given directory.
func findMarkdownFiles(root string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and files
		if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Collect markdown files
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}
