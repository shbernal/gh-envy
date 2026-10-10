package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/jq"
	"github.com/cli/go-gh/v2/pkg/jsonpretty"
	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/spf13/cobra"
)

// Exporter renders --json output, optionally through --jq or --template.
type Exporter struct {
	Fields   []string
	JQ       string
	Template string
}

// FlagError is a usage error: it is printed with the command's usage hint.
type FlagError struct{ Err error }

func (e FlagError) Error() string { return e.Err.Error() }
func (e FlagError) Unwrap() error { return e.Err }

// AddJSONFlags adds --json, --jq and --template to cmd. After flag parsing,
// *target is nil unless --json was given.
func AddJSONFlags(cmd *cobra.Command, target **Exporter, fields []string) {
	sorted := slices.Sorted(slices.Values(fields))
	f := cmd.Flags()
	f.StringSlice("json", nil, "Output JSON with the specified `fields`")
	f.StringP("jq", "q", "", "Filter JSON output using a jq `expression`")
	f.StringP("template", "t", "", "Format JSON output using a Go template; see \"gh help formatting\"")

	_ = cmd.RegisterFlagCompletionFunc("json", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		prefix := ""
		if i := strings.LastIndexByte(toComplete, ','); i >= 0 {
			prefix, toComplete = toComplete[:i+1], toComplete[i+1:]
		}
		var out []string
		for _, field := range sorted {
			if strings.HasPrefix(field, toComplete) {
				out = append(out, prefix+field)
			}
		}
		return out, cobra.ShellCompDirectiveNoSpace
	})

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		jsonFields, _ := c.Flags().GetStringSlice("json")
		jqExpr, _ := c.Flags().GetString("jq")
		tmpl, _ := c.Flags().GetString("template")
		if !c.Flags().Changed("json") {
			switch {
			case jqExpr != "":
				return FlagError{errors.New("cannot use `--jq` without specifying `--json`")}
			case tmpl != "":
				return FlagError{errors.New("cannot use `--template` without specifying `--json`")}
			}
			*target = nil
			return nil
		}
		if web := c.Flags().Lookup("web"); web != nil && web.Changed {
			return FlagError{errors.New("cannot use `--web` with `--json`")}
		}
		if jqExpr != "" && tmpl != "" {
			return FlagError{errors.New("cannot use `--jq` with `--template`")}
		}
		for _, field := range jsonFields {
			if !slices.Contains(fields, field) {
				return FlagError{fmt.Errorf("Unknown JSON field: %q\nAvailable fields:\n  %s", field, strings.Join(sorted, "\n  "))} //nolint:staticcheck // same wording as core gh
			}
		}
		*target = &Exporter{Fields: jsonFields, JQ: jqExpr, Template: tmpl}
		return nil
	}

	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if err.Error() == "flag needs an argument: --json" {
			return FlagError{fmt.Errorf("Specify one or more comma-separated fields for `--json`:\n  %s", strings.Join(sorted, "\n  "))} //nolint:staticcheck // same wording as core gh
		}
		return FlagError{err}
	})
}

// Has reports whether field was requested, so commands skip fetching what is not shown.
func (e *Exporter) Has(field string) bool {
	return slices.Contains(e.Fields, field)
}

// Pick keeps the requested fields of a fully populated record.
func (e *Exporter) Pick(full map[string]any) map[string]any {
	out := make(map[string]any, len(e.Fields))
	for _, f := range e.Fields {
		out[f] = full[f]
	}
	return out
}

// Write renders data (already reduced with Pick) to io.Out.
func (e *Exporter) Write(io *IO, data any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(data); err != nil {
		return err
	}
	switch {
	case e.JQ != "":
		indent := ""
		if io.StdoutTTY {
			indent = "  "
		}
		return jq.EvaluateFormatted(&buf, io.Out, e.JQ, indent, io.ColorEnabled)
	case e.Template != "":
		t := template.New(io.Out, io.Width, io.ColorEnabled)
		if err := t.Parse(e.Template); err != nil {
			return err
		}
		if err := t.Execute(&buf); err != nil {
			return err
		}
		return t.Flush()
	case io.StdoutTTY:
		return jsonpretty.Format(io.Out, &buf, "  ", io.ColorEnabled)
	}
	_, err := io.Out.Write(buf.Bytes())
	return err
}
