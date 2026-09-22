package main

import (
	"fmt"
	"io"
	"os"

	"github.com/xarunoba/taskrunner/internal/cli"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return cli.Execute(args, stdin, stdout, stderr)
}
