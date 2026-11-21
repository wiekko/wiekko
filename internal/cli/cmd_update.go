// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"nexmap.net/internal/buildinfo"
	"nexmap.net/internal/cli/config"
	"nexmap.net/internal/cli/output"
	"nexmap.net/internal/cli/update"
)

// The background update check runs at most once per interval and never
// delays a command by more than budget, measured from its start.
const (
	updateCheckInterval = 24 * time.Hour
	updateCheckBudget   = 2 * time.Second
	updateCheckFile     = "update-check.json"
	downloadTimeout     = 5 * time.Minute
)

func (a *app) updateClient() *update.Client {
	return &update.Client{Base: a.io.Getenv(config.EnvReleasesURL), UserAgent: userAgent()}
}

func (a *app) executable() (string, error) {
	exe := a.io.Executable
	if exe == nil {
		exe = os.Executable
	}
	p, err := exe()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return p, nil
}

// installedWithGoInstall reports whether the binary came from `go install
// module@version`: the toolchain then records the module version but no VCS
// data. Release builds use -buildvcs=false and carry no module version, and
// local builds in a checkout carry VCS data.
func installedWithGoInstall() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return false
	}
	for _, s := range info.Settings {
		if s.Key == "vcs" {
			return false
		}
	}
	return true
}

func updateCommand() *command {
	var check, force bool
	return &command{
		name:    "update",
		args:    "[VERSION]",
		summary: "update nexmap to the newest release, or to VERSION; --check only reports",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&check, "check", false, "only report whether a newer release exists")
			fs.BoolVar(&force, "force", false, "reinstall the same version, or install one the configured server does not support")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) > 1 {
				return usageErr(fmt.Errorf("update takes at most one argument, the version"))
			}
			want := ""
			if len(args) == 1 {
				want = args[0]
				if !update.Valid(want) {
					return usageErr(fmt.Errorf("%q is not a release version like v1.2.3", want))
				}
			}
			current := buildinfo.String()
			uc := a.updateClient()

			ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
			defer cancel()
			var target *update.Release
			var err error
			if want == "" {
				target, err = uc.Latest(ctx)
			} else {
				target, err = uc.Release(ctx, want)
			}
			if err != nil {
				return err
			}

			if check {
				available := update.Compare(target.Version, current) > 0
				if want != "" {
					available = target.Version != current
				}
				data := struct {
					Current         string `json:"current"`
					Latest          string `json:"latest"`
					UpdateAvailable bool   `json:"update_available"`
				}{current, target.Version, available}
				label := "Latest"
				if want != "" {
					label = "Requested"
				}
				return output.Record(a.io.Stdout, a.format, "Update", []output.Field{
					{Label: "Installed", Value: current},
					{Label: label, Value: target.Version},
					{Label: "Update available", Value: yesNo(available)},
				}, data)
			}

			if installedWithGoInstall() {
				return usageErr(fmt.Errorf("this nexmap was installed with `go install`; update it the same way, e.g. `go install nexmap.net/cmd/nexmap@%s`", target.Version))
			}
			if want == "" && !update.Valid(current) {
				return usageErr(fmt.Errorf("this is a development build (%s); pass the version to install, e.g. `nexmap update %s`", current, target.Version))
			}
			if !force && (target.Version == current || (want == "" && update.Compare(target.Version, current) < 0)) {
				fmt.Fprintf(a.io.Stderr, "nexmap %s is up to date\n", current)
				return nil
			}
			if !force {
				if err := a.checkServerSupports(ctx, target); err != nil {
					return err
				}
			}

			exe, err := a.executable()
			if err != nil {
				return fmt.Errorf("locate the running nexmap: %w", err)
			}
			update.CleanupOld(exe)
			bin, err := uc.Download(ctx, target.Version, runtime.GOOS, runtime.GOARCH)
			if err != nil {
				return err
			}
			if err := update.Replace(exe, bin); err != nil {
				if errors.Is(err, fs.ErrPermission) {
					return fmt.Errorf("cannot replace %s: %w; run the update with the rights to write there (e.g. sudo), or reinstall nexmap into a directory you own", exe, err)
				}
				return fmt.Errorf("replace %s: %w", exe, err)
			}
			_ = update.SaveState(a.updateStateFile(), update.State{CheckedAt: time.Now(), Latest: target.Version})

			data := struct {
				Previous string `json:"previous"`
				Version  string `json:"version"`
				Path     string `json:"path"`
			}{current, target.Version, exe}
			return output.Record(a.io.Stdout, a.format, "Updated", []output.Field{
				{Label: "Previous", Value: current},
				{Label: "Installed", Value: target.Version},
				{Label: "Path", Value: exe},
			}, data)
		},
	}
}

// checkServerSupports refuses a release that needs a higher API level than
// the configured server implements. Without a server, or when it cannot be
// reached, there is nothing to check.
func (a *app) checkServerSupports(ctx context.Context, target *update.Release) error {
	if a.settings.Server == "" {
		return nil
	}
	c, err := a.newClient(false)
	if err != nil {
		return nil
	}
	v, err := c.Version(ctx)
	if err != nil || v.APILevel >= target.APILevel {
		return nil
	}
	return usageErr(fmt.Errorf("nexmap %s needs API level %d, but the server at %s implements %d; upgrade the server first, or pass --force",
		target.Version, target.APILevel, c.Server(), v.APILevel))
}

func (a *app) updateStateFile() string {
	dir, err := config.UserDir(a.io.Getenv)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, updateCheckFile)
}

// updateCheck is a background lookup of the newest release; see
// startUpdateCheck.
type updateCheck struct {
	deadline time.Time
	done     chan struct{}
	notice   string
}

// startUpdateCheck looks for a newer release in the background, at most once
// a day. It stays off wherever an extra line on stderr could get in the way:
// when stderr is not a terminal, in CI, for the MCP server, for development
// builds, and when NEXMAP_NO_UPDATE_CHECK is set. It returns nil when no
// check runs.
func (a *app) startUpdateCheck(ctx context.Context, cmd *command) *updateCheck {
	getenv := a.io.Getenv
	current := buildinfo.String()
	if !a.io.StderrIsTTY || cmd.name == "mcp" || cmd.name == "update" ||
		getenv("CI") != "" || config.Truthy(getenv(config.EnvNoUpdateCheck)) || !update.Valid(current) {
		return nil
	}
	file := a.updateStateFile()
	if file == "" {
		return nil
	}
	now := time.Now()
	if last := update.LoadState(file).CheckedAt; now.Sub(last) < updateCheckInterval && !last.After(now) {
		return nil
	}

	u := &updateCheck{deadline: now.Add(updateCheckBudget), done: make(chan struct{})}
	uc := a.updateClient()
	go func() {
		defer close(u.done)
		ctx, cancel := context.WithDeadline(ctx, u.deadline)
		defer cancel()
		r, err := uc.Latest(ctx)
		if err != nil {
			return // silent; the next run tries again
		}
		_ = update.SaveState(file, update.State{CheckedAt: time.Now(), Latest: r.Version})
		if update.Compare(r.Version, current) > 0 {
			u.notice = fmt.Sprintf("nexmap: %s is available (you have %s); run `nexmap update`", r.Version, current)
		}
	}()
	return u
}

// finish waits for the check until its deadline and prints its notice, if
// any. A check still running then is dropped.
func (u *updateCheck) finish(a *app) {
	if u == nil {
		return
	}
	select {
	case <-u.done:
	case <-time.After(time.Until(u.deadline)):
		return
	}
	if u.notice != "" {
		fmt.Fprintln(a.io.Stderr, u.notice)
	}
}
