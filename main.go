// Command gh-envy is a gh extension that manages GitHub deployment environments.
package main

import (
	"os"

	"github.com/shbernal/gh-envy/internal/cmd"
	"github.com/shbernal/gh-envy/internal/cmdutil"
)

func main() {
	os.Exit(cmd.Execute(cmdutil.NewFactory(), os.Args[1:]))
}
