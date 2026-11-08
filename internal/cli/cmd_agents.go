package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// mcpConfigFile is the project-scoped MCP configuration read by Claude Code
// and other MCP clients.
const mcpConfigFile = ".mcp.json"

// mcpServerName is the key of the nexmap entry in mcpConfigFile.
const mcpServerName = "nexmap"

// agentGuideMarker tags the agent guide once it has been appended to an
// instructions file, so repeated `init --agents` runs do not duplicate it.
const agentGuideMarker = "<!-- nexmap:agent-guide -->"

// writeMCPConfig registers `nexmap mcp --read-only` in dir/.mcp.json, keeping
// every other entry. An existing nexmap entry is left alone unless force is
// set. It returns the file and what happened: created, updated or kept.
func writeMCPConfig(dir string, force bool) (string, string, error) {
	path := filepath.Join(dir, mcpConfigFile)
	doc := map[string]json.RawMessage{}
	servers := map[string]json.RawMessage{}
	state := "created"

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		state = "updated"
		if len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &doc); err != nil {
				return path, "", fmt.Errorf("%s is not valid JSON: %w", path, err)
			}
		}
		if raw, ok := doc["mcpServers"]; ok {
			if err := json.Unmarshal(raw, &servers); err != nil {
				return path, "", fmt.Errorf("%s: mcpServers is not an object: %w", path, err)
			}
		}
		if _, ok := servers[mcpServerName]; ok && !force {
			return path, "kept", nil
		}
	case !errors.Is(err, fs.ErrNotExist):
		return path, "", err
	}

	// Read-only until the server has token scopes: an agent with this entry
	// can read the product but not change it.
	entry, _ := json.Marshal(map[string]any{"command": "nexmap", "args": []string{"mcp", "--read-only"}})
	servers[mcpServerName] = entry
	doc["mcpServers"], _ = json.Marshal(servers)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return path, "", err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return path, "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, state, nil
}

// appendAgentGuide appends the agent guide to dir/CLAUDE.md if it exists,
// otherwise to dir/AGENTS.md (created if needed). It returns the file and
// what happened: created, appended or kept (already present).
func appendAgentGuide(dir string) (string, string, error) {
	path := filepath.Join(dir, "AGENTS.md")
	if claude := filepath.Join(dir, "CLAUDE.md"); fileExists(claude) {
		path = claude
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return path, "", err
	}
	if strings.Contains(string(existing), agentGuideMarker) {
		return path, "kept", nil
	}
	state := "created"
	var b strings.Builder
	if len(existing) > 0 {
		state = "appended"
		b.Write(existing)
		if !bytes.HasSuffix(existing, []byte("\n")) {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(agentGuideMarker + "\n")
	b.WriteString(agentGuide)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return path, "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, state, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
