package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"nexmap.net/internal/buildinfo"
	"nexmap.net/internal/cli/config"
	"nexmap.net/internal/cli/output"
	"nexmap.net/internal/client"
)

func versionCommand() *command {
	return &command{
		name:    "version",
		summary: "show the client version and, if a server is configured, the server version",
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) > 0 {
				return usageErr(fmt.Errorf("version takes no arguments"))
			}
			type side struct {
				URL        string `json:"url,omitempty"`
				Version    string `json:"version"`
				APILevel   int    `json:"api_level"`
				Compatible *bool  `json:"compatible,omitempty"`
			}
			data := struct {
				Client side  `json:"client"`
				Server *side `json:"server,omitempty"`
			}{Client: side{Version: buildinfo.String(), APILevel: buildinfo.APILevel}}
			fields := []output.Field{{Label: "Client", Value: fmt.Sprintf("%s (API level %d)", data.Client.Version, data.Client.APILevel)}}

			var serverErr error
			if a.settings.Server != "" {
				c, err := a.newClient(false)
				if err != nil {
					return err
				}
				v, err := c.Version(ctx)
				if err != nil {
					serverErr = err
					fields = append(fields, output.Field{Label: "Server", Value: c.Server() + " — unreachable"})
				} else {
					ok := v.APILevel >= buildinfo.APILevel
					data.Server = &side{URL: c.Server(), Version: v.Version, APILevel: v.APILevel, Compatible: &ok}
					fields = append(fields, output.Field{Label: "Server", Value: fmt.Sprintf("%s (API level %d) at %s", v.Version, v.APILevel, c.Server())})
					if !ok {
						fmt.Fprintf(a.io.Stderr, "warning: the server implements API level %d, this client needs %d; upgrade the server\n", v.APILevel, buildinfo.APILevel)
					}
				}
			}
			if err := output.Record(a.io.Stdout, a.format, "Version", fields, data); err != nil {
				return err
			}
			return serverErr
		},
	}
}

func initCommand() *command {
	var dir string
	var force, agents bool
	return &command{
		name:    "init",
		summary: "create " + config.RepoFileName + " for this repository (server and product); --agents also sets up AI agents",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&dir, "dir", "", "directory to write "+config.RepoFileName+" to (default: repository root)")
			fs.BoolVar(&force, "force", false, "overwrite an existing "+config.RepoFileName+" (and the nexmap entry in "+mcpConfigFile+")")
			fs.BoolVar(&agents, "agents", false, "also register the MCP server in "+mcpConfigFile+" and add the agent guide to CLAUDE.md or AGENTS.md")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) > 0 {
				return usageErr(fmt.Errorf("init takes no arguments"))
			}
			target := dir
			if target == "" {
				target = config.FindRepoRoot(a.io.Cwd)
			} else if !filepath.IsAbs(target) {
				target = filepath.Join(a.io.Cwd, target)
			}
			path := filepath.Join(target, config.RepoFileName)
			exists := false
			if _, err := os.Stat(path); err == nil {
				exists = true
			} else if !errors.Is(err, fs.ErrNotExist) {
				return usageErr(err)
			}
			// With --agents, an existing repository config is kept: setting
			// up agents in an already initialized repository is the common case.
			keep := exists && !force && agents
			if exists && !force && !agents {
				return usageErr(fmt.Errorf("%s already exists (use --force to overwrite)", path))
			}

			fields := []output.Field{}
			result := map[string]any{}
			if keep {
				fields = append(fields, output.Field{Label: "File", Value: path + " (kept)"})
				result["file"] = path
				result["kept"] = true
			} else {
				c, err := a.newClient(true)
				if err != nil {
					return err
				}
				product, err := a.pickProduct(ctx, c)
				if err != nil {
					return err
				}
				repo := &config.Repo{Server: c.Server(), Product: product.ID}
				if err := config.WriteRepo(path, repo); err != nil {
					return fmt.Errorf("write %s: %w", path, err)
				}
				fields = append(fields,
					output.Field{Label: "File", Value: path},
					output.Field{Label: "Server", Value: repo.Server},
					output.Field{Label: "Product", Value: fmt.Sprintf("%s (%s)", product.Name, product.ID)},
				)
				result["file"], result["server"], result["product"] = path, repo.Server, product
			}

			if agents {
				mcpPath, mcpState, err := writeMCPConfig(target, force)
				if err != nil {
					return err
				}
				guidePath, guideState, err := appendAgentGuide(target)
				if err != nil {
					return err
				}
				fields = append(fields,
					output.Field{Label: "MCP", Value: fmt.Sprintf("%s (%s)", mcpPath, mcpState)},
					output.Field{Label: "Guide", Value: fmt.Sprintf("%s (%s)", guidePath, guideState)},
				)
				result["mcp"] = map[string]string{"file": mcpPath, "state": mcpState}
				result["guide"] = map[string]string{"file": guidePath, "state": guideState}
			}
			return output.Record(a.io.Stdout, a.format, "Initialized", fields, result)
		},
	}
}

// pickProduct returns the configured product, the only visible product, or
// asks the user to choose on a terminal.
func (a *app) pickProduct(ctx context.Context, c *client.Client) (*client.Product, error) {
	if id := a.settings.Product; id != "" {
		p, err := c.Product(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("product %s: %w", id, err)
		}
		return p, nil
	}

	products, err := c.Products(ctx)
	if err != nil {
		return nil, err
	}
	switch {
	case len(products) == 0:
		return nil, usageErr(errors.New("you are not a member of any product on this server"))
	case len(products) == 1:
		fmt.Fprintf(a.io.Stderr, "Using the only product available: %s\n", products[0].Name)
		return &products[0], nil
	}

	var list strings.Builder
	for i, p := range products {
		fmt.Fprintf(&list, "  %d) %s  (%s)\n", i+1, p.Name, p.ID)
	}
	if !a.io.StdinIsTTY {
		return nil, usageErr(fmt.Errorf("several products are available; choose one with --product:\n%s", strings.TrimRight(list.String(), "\n")))
	}
	fmt.Fprintf(a.io.Stderr, "Products:\n%sSelect a product [1-%d]: ", list.String(), len(products))
	line, err := bufio.NewReader(a.io.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return nil, usageErr(errors.New("no product selected"))
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(products) {
		return nil, usageErr(fmt.Errorf("invalid selection %q", strings.TrimSpace(line)))
	}
	return &products[n-1], nil
}

func apiCommand() *command {
	var data string
	return &command{
		name:    "api",
		args:    "[METHOD] PATH",
		summary: "send an authenticated request to /api/v1 and print the response ({product} in PATH is replaced)",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&data, "data", "", "request body: JSON text, @file, or @- for stdin")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			method, path := http.MethodGet, ""
			switch len(args) {
			case 1:
				path = args[0]
			case 2:
				method, path = strings.ToUpper(args[0]), args[1]
			default:
				return usageErr(errors.New("usage: nexmap api [METHOD] PATH [--data JSON|@file|@-]"))
			}
			if strings.Contains(path, "{product}") {
				id, err := a.requireProduct()
				if err != nil {
					return err
				}
				path = strings.ReplaceAll(path, "{product}", id)
			}

			var body []byte
			if data != "" {
				var err error
				if body, err = a.readData(data); err != nil {
					return usageErr(err)
				}
				if !json.Valid(body) {
					return usageErr(errors.New("--data is not valid JSON"))
				}
			}

			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			resp, err := c.Raw(ctx, method, path, body, "application/json")
			if err != nil {
				return err
			}
			if err := writeBody(a.io.Stdout, resp.Body); err != nil {
				return err
			}
			if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				return nil
			}
			apiErr := client.ParseError(resp)
			fmt.Fprintf(a.io.Stderr, "nexmap: %s\n", apiErr.Error())
			if resp.StatusCode == http.StatusNotFound {
				return silentExit(ExitNotFound)
			}
			return silentExit(ExitAPI)
		},
	}
}

// readData resolves a --data argument.
func (a *app) readData(arg string) ([]byte, error) {
	switch {
	case arg == "@-":
		return io.ReadAll(a.io.Stdin)
	case strings.HasPrefix(arg, "@"):
		p := arg[1:]
		if !filepath.IsAbs(p) {
			p = filepath.Join(a.io.Cwd, p)
		}
		return os.ReadFile(p)
	default:
		return []byte(arg), nil
	}
}

// writeBody prints a response body, indenting JSON for readability.
func writeBody(w io.Writer, body []byte) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if json.Valid(trimmed) && json.Indent(&buf, trimmed, "", "  ") == nil {
		buf.WriteByte('\n')
		_, err := w.Write(buf.Bytes())
		return err
	}
	_, err := w.Write(append(trimmed, '\n'))
	return err
}
