// Package junit parses JUnit XML reports as written by pytest, jest-junit,
// Maven Surefire, gotestsum and most other runners.
package junit

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"nexmap.net/internal/ingest/report"
)

// suite covers both <testsuites> and <testsuite>; suites may nest.
type suite struct {
	XMLName   xml.Name
	Name      string     `xml:"name,attr"`
	Suites    []suite    `xml:"testsuite"`
	Cases     []testcase `xml:"testcase"`
	SystemOut string     `xml:"system-out"`
	SystemErr string     `xml:"system-err"`
}

type testcase struct {
	Name      string    `xml:"name,attr"`
	Classname string    `xml:"classname,attr"`
	Time      string    `xml:"time,attr"`
	Failures  []problem `xml:"failure"`
	Errors    []problem `xml:"error"`
	Skipped   *problem  `xml:"skipped"`
	SystemOut string    `xml:"system-out"`
	SystemErr string    `xml:"system-err"`
}

type problem struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

// Parse parses one JUnit XML document. With qualified, identifiers are
// "classname.name" (gotestsum: "pkg/path.TestName"; pytest:
// "tests.test_mod.test_x"); otherwise the bare name. When classname and name
// are equal (jest-junit's default) the name is used alone.
func Parse(r io.Reader, qualified bool) (*report.Report, error) {
	var root suite
	if err := xml.NewDecoder(r).Decode(&root); err != nil {
		return nil, err
	}
	switch root.XMLName.Local {
	case "testsuites", "testsuite":
	default:
		return nil, fmt.Errorf("root element is <%s>, want <testsuites> or <testsuite>", root.XMLName.Local)
	}
	rep := &report.Report{}
	walk(&root, qualified, rep)
	return rep, nil
}

func walk(s *suite, qualified bool, rep *report.Report) {
	for _, c := range s.Cases {
		rep.Results = append(rep.Results, result(c, qualified))
	}
	for i := range s.Suites {
		walk(&s.Suites[i], qualified, rep)
	}
}

func result(c testcase, qualified bool) report.Result {
	id := c.Name
	if qualified && c.Classname != "" && c.Classname != c.Name {
		id = c.Classname + "." + c.Name
	}
	res := report.Result{Identifier: id, Status: report.Passed}
	if secs, err := strconv.ParseFloat(c.Time, 64); err == nil {
		res.DurationMS = int64(secs * 1000)
	}
	problems := append(append([]problem{}, c.Failures...), c.Errors...)
	switch {
	case len(problems) > 0:
		res.Status = report.Failed
		var b strings.Builder
		for _, p := range problems {
			if p.Message != "" && !strings.Contains(p.Body, p.Message) {
				b.WriteString(p.Message)
				b.WriteString("\n")
			}
			if body := strings.Trim(p.Body, "\n"); body != "" {
				b.WriteString(body)
				b.WriteString("\n")
			}
		}
		for _, out := range []struct{ label, text string }{{"stdout", c.SystemOut}, {"stderr", c.SystemErr}} {
			if t := strings.TrimSpace(out.text); t != "" {
				fmt.Fprintf(&b, "--- %s ---\n%s\n", out.label, t)
			}
		}
		res.Output = b.String()
	case c.Skipped != nil:
		res.Status = report.Skipped
	}
	return res
}
