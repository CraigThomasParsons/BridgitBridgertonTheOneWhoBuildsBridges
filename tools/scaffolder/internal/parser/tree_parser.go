// Package parser transforms tree-format text files into structured Node objects.
//
// This parser handles the visual tree format produced by the Unix `tree` command
// or hand-written directory structures. It uses regex patterns to strip tree
// drawing characters (│, ├, └, ─) and reconstruct full paths from indentation depth.
package parser

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Node represents a single directory or file in the parsed tree structure.
//
// Each node captures the full path from the tree root and whether it represents
// a directory (indicated by a trailing slash in the tree file).
type Node struct {
	// Path is the slash-separated path from the tree root to this node.
	// Example: "src/internal/parser/tree_parser.go"
	Path string

	// IsDir indicates whether this node is a directory (true) or file (false).
	// Determined by checking for a trailing slash in the cleaned name.
	IsDir bool
}

// Regex patterns for parsing tree drawing characters and depth.
// These are compiled once at package init for performance.
var (
	// prefixRegex matches the indentation prefix (│ or spaces in groups of 4).
	// Used to calculate tree depth by counting 4-character groups.
	prefixRegex = regexp.MustCompile(`^([│ ]{4})*`)

	// cleanRegex matches tree drawing characters at the start of lines.
	// Removes │, ├, └, ─, and spaces to extract the actual name.
	cleanRegex = regexp.MustCompile(`^[│ ├└─]+`)

	// commentRegex matches parenthetical comments at the end of lines.
	// Example: "file.go (contains parser logic)" → "file.go"
	commentRegex = regexp.MustCompile(`\s*\(.*\)$`)
)

// Parse reads a tree-format file and returns a slice of Node objects.
//
// The parser reads line-by-line, calculates depth from indentation, strips
// tree drawing characters, and reconstructs full paths using a depth stack.
// Blank lines and lines that clean to empty strings are skipped.
//
// Example tree input:
//
//	src/
//	│   main.go
//	│   parser/
//	│   │   tree_parser.go
//
// Produces nodes with paths: "src/", "src/main.go", "src/parser/", "src/parser/tree_parser.go"
func Parse(path string) ([]Node, error) {
	// Open the tree file for reading.
	// File handle will be closed via defer to ensure cleanup.
	file, err := os.Open(path)
	if err != nil {
		// File not found or permission denied.
		// Cannot proceed without the tree specification.
		return nil, err
	}
	defer file.Close()

	// Create a line scanner for memory-efficient reading.
	// This allows handling large tree files without loading everything into memory.
	scanner := bufio.NewScanner(file)

	// Pre-allocate slices for nodes and the path stack.
	var nodes []Node

	// Stack tracks the current path components based on depth.
	// As we descend/ascend the tree, we push/pop from this stack.
	var stack []string

	// Process each line of the tree file.
	for scanner.Scan() {
		line := scanner.Text()

		// Skip blank lines - they're just formatting.
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Calculate the depth (nesting level) from indentation.
		// Each 4-character group represents one level of depth.
		depth := parseDepth(line)

		// Extract the cleaned name by removing tree drawing characters.
		// This also strips comments and extra whitespace.
		name := cleanName(line)

		// Skip lines that clean to nothing (e.g., all tree chars).
		if name == "" {
			continue
		}

		// Adjust the stack to the current depth.
		// This pops off deeper levels when we return to a shallower depth.
		stack = stack[:depth]

		// Push the current name onto the stack.
		// The stack now represents the full path to this node.
		stack = append(stack, name)

		// Reconstruct the full path by joining stack components with slashes.
		// Example: ["src", "parser", "tree_parser.go"] → "src/parser/tree_parser.go"
		fullPath := strings.Join(stack, "/")

		// Create a Node with the full path and directory flag.
		// Trailing slash in the name indicates a directory.
		nodes = append(nodes, Node{
			Path:  fullPath,
			IsDir: strings.HasSuffix(name, "/"),
		})
	}

	// Check for scanner errors that may have occurred during reading.
	// Without this check, truncated tree files would silently produce partial results.
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Return all parsed nodes.
	return nodes, nil
}

// parseDepth calculates the nesting depth of a tree line based on indentation.
//
// Tree format uses 4-character groups (│ or spaces) for each level of depth.
// This function counts those groups using the prefixRegex pattern.
//
// Example: "│   │   file.go" has 2 groups → depth 2
func parseDepth(line string) int {
	// Extract the prefix portion (all indentation/tree chars before the name).
	match := prefixRegex.FindString(line)

	// Count runes instead of bytes because tree drawing characters (│, ├, └, ─)
	// are multi-byte in UTF-8 (3 bytes each). Using len() would over-count depth
	// and produce corrupted paths like "internal/builder/main.go/filesystem.go".
	return utf8.RuneCountInString(match) / 4
}

// cleanName removes tree drawing characters and comments from a tree line.
//
// This function applies three transformations in order:
// 1. Strip leading tree drawing characters (│, ├, └, ─)
// 2. Trim surrounding whitespace
// 3. Remove trailing parenthetical comments
//
// Example: "├── src/ (source code)" → "src/"
func cleanName(line string) string {
	// Remove all tree drawing characters from the start.
	name := cleanRegex.ReplaceAllString(line, "")

	// Trim any remaining whitespace around the name.
	name = strings.TrimSpace(name)

	// Remove trailing comments in parentheses.
	// This allows annotated tree files without polluting the filesystem.
	name = commentRegex.ReplaceAllString(name, "")

	return name
}
