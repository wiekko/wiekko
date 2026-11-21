// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"nexmap.net/internal/cli/config"
	"nexmap.net/internal/cli/output"
	"nexmap.net/internal/client"
)

func loginCommand() *command {
	return &command{
		name:    "login",
		summary: "store a personal access token for the server",
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) > 0 {
				return usageErr(fmt.Errorf("login takes no arguments; the token is read from stdin"))
			}
			server := a.settings.Server
			if server == "" {
				return usageErr(fmt.Errorf("no server configured: pass --server, set %s, or add it to %s", config.EnvServer, config.RepoFileName))
			}
			norm, err := client.NormalizeServer(server)
			if err != nil {
				return usageErr(err)
			}

			token, err := a.readToken(norm)
			if err != nil {
				return err
			}
			c, err := a.clientFor(norm, token, true)
			if err != nil {
				return err
			}
			me, err := c.Me(ctx)
			if err != nil {
				if client.IsUnauthorized(err) {
					return &exitError{code: ExitAPI, err: errors.New("the server rejected the token (invalid, expired or revoked)")}
				}
				return err
			}

			if err := a.settings.User.SetToken(norm, token, me.Username); err != nil {
				return usageErr(err)
			}
			if err := config.SaveUser(a.settings.UserFile, a.settings.User); err != nil {
				return fmt.Errorf("save credentials: %w", err)
			}
			if a.io.Getenv(config.EnvToken) != "" {
				fmt.Fprintf(a.io.Stderr, "warning: %s is set and takes precedence over the stored token\n", config.EnvToken)
			}
			return output.Record(a.io.Stdout, a.format, "Logged in", []output.Field{
				{Label: "Server", Value: norm},
				{Label: "User", Value: me.Username},
				{Label: "Credentials", Value: a.settings.UserFile},
			}, map[string]any{"server": norm, "username": me.Username, "credentials_file": a.settings.UserFile})
		},
	}
}

// readToken reads the PAT: hidden prompt on a terminal, else the first line
// of stdin (`echo $TOKEN | nexmap login`). There is deliberately no flag for
// the token itself, which would leak via shell history and process listings.
func (a *app) readToken(server string) (string, error) {
	var token string
	if a.io.StdinIsTTY && a.io.ReadSecret != nil {
		fmt.Fprintf(a.io.Stderr, "Create a token at %s/user/tokens\nPersonal access token: ", server)
		t, err := a.io.ReadSecret()
		fmt.Fprintln(a.io.Stderr)
		if err != nil {
			return "", usageErr(fmt.Errorf("read token: %w", err))
		}
		token = t
	} else {
		line, err := bufio.NewReader(a.io.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", usageErr(fmt.Errorf("read token from stdin: no input"))
		}
		token = line
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", usageErr(errors.New("empty token"))
	}
	return token, nil
}

func logoutCommand() *command {
	return &command{
		name:    "logout",
		summary: "remove the stored token for the server",
		run: func(_ context.Context, a *app, args []string) error {
			if len(args) > 0 {
				return usageErr(fmt.Errorf("logout takes no arguments"))
			}
			if a.settings.Server == "" {
				return usageErr(fmt.Errorf("no server configured: pass --server or set %s", config.EnvServer))
			}
			norm, err := client.NormalizeServer(a.settings.Server)
			if err != nil {
				return usageErr(err)
			}
			removed, err := a.settings.User.RemoveToken(norm)
			if err != nil {
				return usageErr(err)
			}
			if !removed {
				fmt.Fprintf(a.io.Stderr, "no stored token for %s\n", norm)
				return nil
			}
			if err := config.SaveUser(a.settings.UserFile, a.settings.User); err != nil {
				return fmt.Errorf("save credentials: %w", err)
			}
			fmt.Fprintf(a.io.Stderr, "Removed the stored token for %s.\nThe token itself stays valid until you revoke it at %s/user/tokens\n", norm, norm)
			return nil
		},
	}
}

func whoamiCommand() *command {
	return &command{
		name:    "whoami",
		summary: "show the authenticated user, server and product in effect",
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) > 0 {
				return usageErr(fmt.Errorf("whoami takes no arguments"))
			}
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			me, err := c.Me(ctx)
			if err != nil {
				return err
			}

			data := map[string]any{
				"server":       c.Server(),
				"server_from":  a.settings.ServerSource,
				"user":         me,
				"token_from":   a.settings.TokenSource,
				"product":      nil,
				"product_from": a.settings.ProductSource,
			}
			fields := []output.Field{
				{Label: "User", Value: me.Username},
				{Label: "Display name", Value: me.DisplayName},
				{Label: "Administrator", Value: yesNo(me.IsAdmin)},
				{Label: "Server", Value: withSource(c.Server(), a.settings.ServerSource)},
			}

			productValue := ""
			if id := a.settings.Product; id != "" {
				p, err := c.Product(ctx, id)
				switch {
				case err == nil:
					data["product"] = p
					productValue = fmt.Sprintf("%s (%s)", p.Name, p.ID)
				case client.IsNotFound(err) || client.IsForbidden(err):
					productValue = id + " — not found or no access"
				default:
					return err
				}
				productValue = withSource(productValue, a.settings.ProductSource)
			}
			fields = append(fields,
				output.Field{Label: "Product", Value: productValue},
				output.Field{Label: "Token from", Value: a.settings.TokenSource},
			)
			if t := me.Token; t != nil {
				scopes := strings.Join(t.Scopes, " ")
				if scopes == "" {
					scopes = "none"
				}
				roleCap := t.RoleCap
				if roleCap == "" {
					roleCap = "none"
				}
				products := "all"
				if t.Products != nil {
					products = strings.Join(t.Products, ", ")
					if products == "" {
						products = "none"
					}
				}
				fields = append(fields,
					output.Field{Label: "Token scopes", Value: scopes},
					output.Field{Label: "Token role cap", Value: roleCap},
					output.Field{Label: "Token products", Value: products},
				)
			}
			return output.Record(a.io.Stdout, a.format, "Identity", fields, data)
		},
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func withSource(value, source string) string {
	if value == "" || source == "" {
		return value
	}
	return fmt.Sprintf("%s  [%s]", value, source)
}
