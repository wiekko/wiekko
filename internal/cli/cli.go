// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Package cli implements the nexmap command line client. It talks to a Nexmap
// server exclusively through internal/client (AC-16) and never imports server
// packages.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"

	"nexmap.net/internal/buildinfo"
	"nexmap.net/internal/cli/config"
	"nexmap.net/internal/cli/ops"
	"nexmap.net/internal/cli/output"
	"nexmap.net/internal/client"
)

// Exit codes. They are part of the CLI contract for scripts and agents.
const (
	ExitOK          = 0
	ExitUsage       = 1 // usage or configuration error
	ExitAPI         = 2 // server unreachable, authentication or other API error
	ExitNotFound    = 3 // the requested entity does not exist (or is not visible)
	ExitTestsFailed = 4 // `utest --exit-code`: a test failed
)

// IO is the process environment a command runs in. Tests construct it
// directly; main builds it from the real process.
type IO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string
	Cwd    string

	StdinIsTTY bool
	// ReadSecret reads one line from the terminal without echo. Only called
	// when StdinIsTTY is true.
	ReadSecret func() (string, error)

	// StderrIsTTY enables the daily update notice, which is shown on a
	// terminal only.
	StderrIsTTY bool
	// Executable returns the path of the running binary, for `update`; nil
	// means os.Executable.
	Executable func() (string, error)
}

// globals are the flags accepted by every command.
type globals struct {
	server    string
	product   string
	output    string
	allowHTTP bool
}

func (g *globals) bind(fs *flag.FlagSet) {
	fs.StringVar(&g.server, "server", g.server, "Nexmap server URL (env "+config.EnvServer+")")
	fs.StringVar(&g.product, "product", g.product, "product ID (env "+config.EnvProduct+")")
	fs.StringVar(&g.output, "output", g.output, "output format: table, md or json (env "+config.EnvOutput+")")
	fs.StringVar(&g.output, "o", g.output, "shorthand for --output")
	fs.BoolVar(&g.allowHTTP, "allow-http", g.allowHTTP, "allow plain HTTP to non-local servers (env "+config.EnvAllowHTTP+")")
}

// app is the state shared by the commands of one invocation.
type app struct {
	io      IO
	globals globals

	settings *config.Settings
	format   output.Format

	warnedTokenDestination bool
}

type command struct {
	name    string
	args    string // positional argument synopsis
	summary string
	// flags registers command-specific flags; may be nil.
	flags func(fs *flag.FlagSet)
	run   func(ctx context.Context, a *app, args []string) error
	// subs makes this a command group ("req list", "req show"); run is
	// unused then.
	subs []*command
}

func commands() []*command {
	return []*command{
		versionCommand(),
		updateCommand(),
		loginCommand(),
		logoutCommand(),
		whoamiCommand(),
		initCommand(),
		apiCommand(),
		contextCommand(),
		requirementGroup("req", "requirements and tasks", ""),
		requirementGroup("task", "tasks (shorthand for `req … --kind task`)", "task"),
		constraintGroup(),
		documentGroup(),
		componentGroup(),
		capabilityGroup(),
		defectGroup(),
		testGroup(),
		campaignGroup(),
		commentGroup(),
		utestCommand(),
		exportCommand(),
		agentGuideCommand(),
		mcpCommand(),
	}
}

// Run executes the CLI with args (without the program name) and returns the
// process exit code.
func Run(ctx context.Context, args []string, io IO) int {
	a := &app{io: io}

	// Global flags may precede the command name.
	top := flag.NewFlagSet("nexmap", flag.ContinueOnError)
	top.SetOutput(io.Stderr)
	a.globals.bind(top)
	top.Usage = func() { a.usage(io.Stderr) }
	if err := top.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitUsage
	}
	rest := top.Args()
	if len(rest) == 0 {
		a.usage(io.Stderr)
		return ExitUsage
	}

	name := rest[0]
	if name == "help" {
		if len(rest) > 1 {
			if cmd := lookup(rest[1]); cmd != nil {
				if len(rest) > 2 {
					if sub := cmd.sub(rest[2]); sub != nil {
						cmd = sub
					}
				}
				a.commandUsage(io.Stdout, cmd, nil)
				return ExitOK
			}
		}
		a.usage(io.Stdout)
		return ExitOK
	}
	cmd := lookup(name)
	if cmd == nil {
		fmt.Fprintf(io.Stderr, "nexmap: unknown command %q\n\n", name)
		a.usage(io.Stderr)
		return ExitUsage
	}
	rest = rest[1:]
	if len(cmd.subs) > 0 {
		if len(rest) == 0 {
			a.commandUsage(io.Stderr, cmd, nil)
			return ExitUsage
		}
		if rest[0] == "-h" || rest[0] == "--help" || rest[0] == "help" {
			a.commandUsage(io.Stdout, cmd, nil)
			return ExitOK
		}
		sub := cmd.sub(rest[0])
		if sub == nil {
			fmt.Fprintf(io.Stderr, "nexmap: unknown command %q\n\n", cmd.name+" "+rest[0])
			a.commandUsage(io.Stderr, cmd, nil)
			return ExitUsage
		}
		cmd, rest = sub, rest[1:]
	}

	// Global flags are accepted after the command name as well.
	fs := flag.NewFlagSet("nexmap "+cmd.name, flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	a.globals.bind(fs)
	if cmd.flags != nil {
		cmd.flags(fs)
	}
	fs.Usage = func() { a.commandUsage(io.Stderr, cmd, fs) }
	positional, err := parseInterleaved(fs, rest)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitUsage
	}

	if err := a.setup(); err != nil {
		return a.fail(err)
	}
	check := a.startUpdateCheck(ctx, cmd)
	code := ExitOK
	if err := cmd.run(ctx, a, positional); err != nil {
		code = a.fail(err)
	}
	check.finish(a)
	return code
}

// sub returns the group member "<group> <name>", if any.
func (c *command) sub(name string) *command {
	for _, s := range c.subs {
		if s.name == c.name+" "+name {
			return s
		}
	}
	return nil
}

func lookup(name string) *command {
	for _, c := range commands() {
		if c.name == name {
			return c
		}
	}
	return nil
}

// parseInterleaved parses flags that may appear between positional arguments
// ("api GET /x --data @f"), which the flag package alone does not support.
// Everything after a literal "--" is positional.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		consumed := len(args) - len(rest)
		if consumed > 0 && args[consumed-1] == "--" {
			return append(positional, rest...), nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// setup resolves configuration once flags are known.
func (a *app) setup() error {
	s, err := config.Resolve(a.io.Cwd, a.io.Getenv, config.Overrides{
		Server:    a.globals.server,
		Product:   a.globals.product,
		Output:    a.globals.output,
		AllowHTTP: a.globals.allowHTTP,
	})
	if err != nil {
		return usageErr(err)
	}
	a.settings = s
	if a.format, err = output.ParseFormat(s.Output); err != nil {
		return usageErr(err)
	}
	return nil
}

// newClient returns an API client for the resolved server. withToken requires
// credentials and fails with a hint to log in when there are none.
func (a *app) newClient(withToken bool) (*client.Client, error) {
	if withToken {
		a.warnTokenDestination()
	}
	return a.clientFor(a.settings.Server, a.settings.Token, withToken)
}

// warnTokenDestination points out, once per run, that the token from the
// environment is about to go to a server chosen by the repository's
// .nexmap.json. Stored tokens are kept per server, so a changed file cannot
// redirect them; the environment's token goes wherever the file says. In CI a
// branch that edits the file could thus collect the job's token, which is why
// the server should be set next to the token there.
func (a *app) warnTokenDestination() {
	s := a.settings
	if a.warnedTokenDestination || s.TokenSource != config.EnvToken || s.RepoFile == "" || s.ServerSource != s.RepoFile {
		return
	}
	a.warnedTokenDestination = true
	fmt.Fprintf(a.io.Stderr, "nexmap: warning: sending the token from %s to %s, the server named in %s. Set %s as well, so that a changed file cannot redirect the token.\n",
		config.EnvToken, s.Server, s.RepoFile, config.EnvServer)
}

func (a *app) clientFor(server, token string, withToken bool) (*client.Client, error) {
	if server == "" {
		return nil, usageErr(fmt.Errorf("no server configured: pass --server, set %s, or run `nexmap init`", config.EnvServer))
	}
	opts := []client.Option{client.WithUserAgent(userAgent())}
	if withToken {
		opts = append(opts, client.WithToken(token))
	}
	if a.settings.AllowHTTP {
		opts = append(opts, client.AllowInsecure())
	}
	c, err := client.New(server, opts...)
	if err != nil {
		if errors.Is(err, client.ErrInsecureServer) {
			err = fmt.Errorf("%w: pass --allow-http or set %s=1", err, config.EnvAllowHTTP)
		}
		return nil, usageErr(err)
	}
	// Checked after client.New so an unusable server URL is reported first.
	if withToken && token == "" {
		return nil, usageErr(fmt.Errorf("not logged in to %s: run `nexmap login` or set %s", c.Server(), config.EnvToken))
	}
	return c, nil
}

// requireProduct returns the resolved product ID or a usage error.
func (a *app) requireProduct() (string, error) {
	if a.settings.Product == "" {
		return "", usageErr(fmt.Errorf("no product configured: pass --product, set %s, or run `nexmap init`", config.EnvProduct))
	}
	return a.settings.Product, nil
}

func userAgent() string {
	return fmt.Sprintf("nexmap/%s (%s/%s)", buildinfo.String(), runtime.GOOS, runtime.GOARCH)
}

// exitError carries an explicit exit code.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func usageErr(err error) error { return &exitError{code: ExitUsage, err: err} }

// silentExit ends the command with code without printing anything further
// (the command already reported what went wrong).
type silentExit int

func (e silentExit) Error() string { return fmt.Sprintf("exit %d", int(e)) }

// fail reports err on stderr and maps it to an exit code.
func (a *app) fail(err error) int {
	var silent silentExit
	if errors.As(err, &silent) {
		return int(silent)
	}

	code := ExitAPI
	var ee *exitError
	var ae *client.APIError
	switch {
	case errors.As(err, &ee):
		code = ee.code
	case client.IsNotFound(err), errors.As(err, new(*ops.NotFoundError)):
		code = ExitNotFound
	case errors.As(err, new(*ops.AmbiguousError)):
		code = ExitUsage
	}

	if a.format == output.JSON {
		body := map[string]string{"message": err.Error()}
		if errors.As(err, &ae) {
			body["code"] = ae.Code
			body["message"] = ae.Message
		}
		_ = output.WriteJSON(a.io.Stderr, map[string]any{"error": body})
		return code
	}
	msg := err.Error()
	if client.IsUnauthorized(err) {
		msg += "\n(the token is missing, invalid, expired or revoked: run `nexmap login`)"
	}
	if client.IsForbidden(err) {
		msg += "\n(not permitted for your role in this product; ask a product owner)"
	}
	fmt.Fprintf(a.io.Stderr, "nexmap: %s\n", msg)
	return code
}

func (a *app) usage(w io.Writer) {
	var b strings.Builder
	b.WriteString("nexmap — command line client for Nexmap\n\n")
	b.WriteString("Usage:\n  nexmap [global flags] <command> [arguments]\n\nCommands:\n")
	cmds := commands()
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].name < cmds[j].name })
	for _, c := range cmds {
		fmt.Fprintf(&b, "  %-12s %s\n", c.name, c.summary)
	}
	b.WriteString("  help         show help for a command, e.g. `nexmap help req show`\n\n")
	b.WriteString("Global flags:\n")
	b.WriteString("  --server URL     Nexmap server (env NEXMAP_SERVER, .nexmap.json)\n")
	b.WriteString("  --product ID     product (env NEXMAP_PRODUCT, .nexmap.json)\n")
	b.WriteString("  -o, --output F   table, md or json (env NEXMAP_OUTPUT)\n")
	b.WriteString("  --allow-http     allow plain HTTP to non-local servers\n\n")
	b.WriteString("Credentials come from NEXMAP_TOKEN or `nexmap login`.\n")
	b.WriteString("AI agents: `nexmap agent-guide` explains usage, `nexmap context` briefs on the product.\n")
	b.WriteString("CI: `go test -json ./... | nexmap utest --env linux` records results in the active campaign.\n")
	b.WriteString("Exit codes: 0 ok, 1 usage/config, 2 API/auth error, 3 not found, 4 tests failed (utest --exit-code).\n")
	_, _ = io.WriteString(w, b.String())
}

func (a *app) commandUsage(w io.Writer, c *command, fs *flag.FlagSet) {
	if len(c.subs) > 0 {
		fmt.Fprintf(w, "Usage: nexmap %s <command> [flags]\n\n%s\n\nCommands:\n", c.name, c.summary)
		for _, s := range c.subs {
			name := strings.TrimPrefix(s.name, c.name+" ")
			if s.args != "" {
				name += " " + s.args
			}
			fmt.Fprintf(w, "  %-20s %s\n", name, s.summary)
		}
		return
	}
	fmt.Fprintf(w, "Usage: nexmap %s", c.name)
	if c.args != "" {
		fmt.Fprintf(w, " %s", c.args)
	}
	fmt.Fprintf(w, " [flags]\n\n%s\n", c.summary)
	if fs == nil {
		fs = flag.NewFlagSet(c.name, flag.ContinueOnError)
		var g globals
		g.bind(fs)
		if c.flags != nil {
			c.flags(fs)
		}
	}
	fmt.Fprintln(w, "\nFlags:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}
