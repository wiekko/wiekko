package config

import (
	"strings"
)

// Overrides are the values given on the command line; empty means "not set".
type Overrides struct {
	Server    string
	Product   string
	Output    string
	AllowHTTP bool
}

// Settings is the resolved configuration for one CLI invocation. The *Source
// fields name where a value came from, for `nexmap whoami` and error messages.
type Settings struct {
	Server        string
	ServerSource  string
	Product       string
	ProductSource string
	Token         string
	TokenSource   string
	Output        string
	AllowHTTP     bool

	RepoFile string // path of the .nexmap.json in effect; "" if none
	UserFile string // path of the user config file (may not exist yet)
	User     *User  // loaded user config, for commands that modify it
}

// Resolve merges user config, repo config, environment and flags. Precedence,
// lowest to highest: user config → repo config → environment → flags. The
// token is looked up for the server that wins, so switching servers via flag
// or environment also switches credentials.
func Resolve(cwd string, getenv func(string) string, o Overrides) (*Settings, error) {
	s := &Settings{}

	userFile, err := UserFile(getenv)
	if err != nil {
		return nil, err
	}
	s.UserFile = userFile
	if s.User, err = LoadUser(userFile); err != nil {
		return nil, err
	}

	repoFile, repo, err := FindRepo(cwd)
	if err != nil {
		return nil, err
	}
	s.RepoFile = repoFile

	set := func(dst, src *string, value, source string) {
		if value != "" {
			*dst, *src = value, source
		}
	}

	var outputSource string
	set(&s.Output, &outputSource, s.User.Output, "user config")
	if repo != nil {
		set(&s.Server, &s.ServerSource, repo.Server, repoFile)
		set(&s.Product, &s.ProductSource, repo.Product, repoFile)
	}
	set(&s.Server, &s.ServerSource, getenv(EnvServer), EnvServer)
	set(&s.Product, &s.ProductSource, getenv(EnvProduct), EnvProduct)
	set(&s.Output, &outputSource, getenv(EnvOutput), EnvOutput)
	set(&s.Server, &s.ServerSource, o.Server, "--server")
	set(&s.Product, &s.ProductSource, o.Product, "--product")
	set(&s.Output, &outputSource, o.Output, "--output")

	s.AllowHTTP = o.AllowHTTP || truthy(getenv(EnvAllowHTTP))

	if t := getenv(EnvToken); t != "" {
		s.Token, s.TokenSource = t, EnvToken
	} else if s.Server != "" {
		if t := s.User.Token(s.Server); t != "" {
			s.Token, s.TokenSource = t, userFile
		}
	}
	return s, nil
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
