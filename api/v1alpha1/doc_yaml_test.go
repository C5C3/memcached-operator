// Package v1alpha1 contains documentation YAML validation tests that verify
// all YAML code blocks in the documentation parse as valid Memcached CRs.
package v1alpha1

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// extractYAMLBlocks parses a markdown file and extracts all YAML code blocks.
// Returns a slice of YAML strings, one for each ```yaml...``` block found.
func extractYAMLBlocks(t *testing.T, filePath string) []string {
	t.Helper()

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filePath, err)
	}

	var blocks []string
	var currentBlock strings.Builder
	inYAMLBlock := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	for scanner.Scan() {
		line := scanner.Text()

		// Check for YAML code fence start
		if strings.HasPrefix(line, "```yaml") {
			inYAMLBlock = true
			currentBlock.Reset()
			continue
		}

		// Check for code fence end
		if inYAMLBlock && strings.HasPrefix(line, "```") {
			inYAMLBlock = false
			blocks = append(blocks, currentBlock.String())
			currentBlock.Reset()
			continue
		}

		// Accumulate lines inside YAML block
		if inYAMLBlock {
			currentBlock.WriteString(line)
			currentBlock.WriteString("\n")
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("error scanning file: %v", err)
	}

	return blocks
}

// filterCompleteYAMLCRs filters YAML blocks to only those that contain apiVersion
// (indicating a complete CR rather than a partial snippet).
func filterCompleteYAMLCRs(blocks []string) []string {
	var complete []string
	for _, block := range blocks {
		if strings.Contains(block, "apiVersion:") {
			complete = append(complete, block)
		}
	}
	return complete
}

// examplesFilePath returns the resolved path to docs/how-to/examples.md
// relative to this test file's location.
func examplesFilePath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get caller info")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "docs", "how-to", "examples.md")
}

// TestDocumentationYAMLExamples verifies that all YAML code blocks in the
// examples documentation parse as valid Memcached CRs with correct
// apiVersion/kind/metadata fields.
func TestDocumentationYAMLExamples(t *testing.T) {
	examplesPath := examplesFilePath(t)

	// Extract all YAML blocks from the markdown file
	allBlocks := extractYAMLBlocks(t, examplesPath)

	// Sanity check: ensure we found YAML blocks
	if len(allBlocks) == 0 {
		t.Fatal("no YAML blocks found in examples.md - file may be empty or malformed")
	}

	// Filter to only complete CRs (those with apiVersion)
	crBlocks := filterCompleteYAMLCRs(allBlocks)

	// Verify we found the expected number of complete CR examples
	expectedCRCount := 7
	if len(crBlocks) != expectedCRCount {
		t.Errorf("expected %d complete CR YAML blocks, found %d", expectedCRCount, len(crBlocks))
	}

	// Track expected CR names to verify we got all of them
	expectedCRs := map[string]bool{
		"memcached-dev":        false,
		"memcached-ha":         false,
		"memcached-monitored":  false,
		"memcached-tls":        false,
		"memcached-sasl":       false,
		"memcached-production": false,
		"keystone-cache":       false,
	}

	// Test each complete CR block
	for i, yamlBlock := range crBlocks {
		var memcached Memcached
		if err := yaml.Unmarshal([]byte(yamlBlock), &memcached); err != nil {
			t.Errorf("block %d: failed to unmarshal YAML: %v\nYAML:\n%s", i+1, err, yamlBlock)
			continue
		}

		testName := memcached.Name
		if testName == "" {
			testName = fmt.Sprintf("unnamed_block_%d", i+1)
		}

		t.Run(testName, func(t *testing.T) {
			// Verify apiVersion
			expectedAPIVersion := "memcached.c5c3.io/v1alpha1"
			if memcached.APIVersion != expectedAPIVersion {
				t.Errorf("apiVersion: got %q, want %q", memcached.APIVersion, expectedAPIVersion)
			}

			// Verify kind
			expectedKind := "Memcached" //nolint:goconst // test literal
			if memcached.Kind != expectedKind {
				t.Errorf("kind: got %q, want %q", memcached.Kind, expectedKind)
			}

			// Verify metadata.name is non-empty
			if memcached.Name == "" {
				t.Error("metadata.name is empty")
			}

			// Mark this CR as found
			if _, exists := expectedCRs[memcached.Name]; exists {
				expectedCRs[memcached.Name] = true
			}

			// Additional validation: verify spec fields are properly typed
			if memcached.Spec.Replicas != nil && *memcached.Spec.Replicas < 0 {
				t.Errorf("spec.replicas is negative: %d", *memcached.Spec.Replicas)
			}
		})
	}

	// Report any missing expected CRs
	var missing []string
	for name, found := range expectedCRs {
		if !found {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("expected CR examples not found in documentation: %v", missing)
	}
}

// TestDocumentationYAMLBlockCount verifies the total number of YAML blocks
// (both complete CRs and partial snippets) matches expectations.
func TestDocumentationYAMLBlockCount(t *testing.T) {
	examplesPath := examplesFilePath(t)

	allBlocks := extractYAMLBlocks(t, examplesPath)

	// As of the current documentation, we expect:
	// - 7 complete CR examples
	// - 1 partial mTLS snippet (showing just the tls: section)
	// Total: 8 YAML blocks
	expectedTotal := 8

	if len(allBlocks) != expectedTotal {
		t.Errorf("expected %d total YAML blocks, found %d", expectedTotal, len(allBlocks))
		t.Logf("This test verifies we haven't accidentally removed or added YAML blocks")
	}
}

// TestDocumentationPartialYAMLSnippets verifies that partial YAML snippets
// (those without apiVersion) are present and contain expected content.
func TestDocumentationPartialYAMLSnippets(t *testing.T) {
	examplesPath := examplesFilePath(t)

	allBlocks := extractYAMLBlocks(t, examplesPath)
	crBlocks := filterCompleteYAMLCRs(allBlocks)

	// Partial snippets are YAML blocks without apiVersion
	partialCount := len(allBlocks) - len(crBlocks)

	// We expect 1 partial snippet (the mTLS tls: section)
	expectedPartialCount := 1
	if partialCount != expectedPartialCount {
		t.Errorf("expected %d partial YAML snippets, found %d", expectedPartialCount, partialCount)
	}

	// Verify the partial snippet contains TLS configuration
	for _, block := range allBlocks {
		if !strings.Contains(block, "apiVersion:") {
			// This is a partial snippet
			if strings.Contains(block, "tls:") && strings.Contains(block, "enableClientCert:") {
				// Found the expected mTLS snippet
				return
			}
		}
	}

	t.Error("expected to find partial mTLS YAML snippet with 'tls:' and 'enableClientCert:' fields")
}

// TestDocumentationNoBashOrIniBlocks verifies that the YAML extraction only
// captures ```yaml blocks and not ```bash or ```ini blocks.
func TestDocumentationNoBashOrIniBlocks(t *testing.T) {
	examplesPath := examplesFilePath(t)

	// Read file and check for bash/ini blocks that shouldn't be in our YAML extraction
	data, err := os.ReadFile(examplesPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	content := string(data)

	// Verify the file actually contains bash/ini blocks (sanity check)
	if !strings.Contains(content, "```bash") {
		t.Error("expected examples.md to contain ```bash blocks for commands")
	}
	if !strings.Contains(content, "```ini") {
		t.Error("expected examples.md to contain ```ini blocks for config examples")
	}

	// Verify our extraction doesn't capture them
	yamlBlocks := extractYAMLBlocks(t, examplesPath)
	for i, block := range yamlBlocks {
		// Bash blocks would contain shell commands
		if strings.Contains(block, "openssl req") || strings.Contains(block, "kubectl create") {
			t.Errorf("block %d appears to contain bash commands (should only extract ```yaml blocks):\n%s", i+1, block)
		}

		// INI blocks would contain [sections]
		if strings.Contains(block, "[cache]") && strings.Contains(block, "dogpile.cache") {
			t.Errorf("block %d appears to contain INI config (should only extract ```yaml blocks):\n%s", i+1, block)
		}
	}
}
