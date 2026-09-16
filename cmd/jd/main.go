package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tangyao927/jd/internal/cli"
	"github.com/tangyao927/jd/internal/config"
	"github.com/tangyao927/jd/internal/store"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	paths, err := config.ResolvePaths()
	if err != nil {
		fmt.Fprintf(stderr, "jd: resolve paths: %v\n", err)
		return cli.ExitRuntime
	}
	cfg := config.Default()
	if cli.RequiresConfig(args) {
		cfg, err = config.Load(paths.ConfigFile)
		if err != nil {
			fmt.Fprintf(stderr, "jd: load config: %v\n", err)
			return cli.ExitRuntime
		}
	}
	database := store.NewLazy(paths.DatabaseFile)
	defer database.Close()
	binaryPath, _ := os.Executable()
	runtime := &cli.Runtime{
		Store: database, Config: cfg, Paths: paths,
		Stdin: stdin, Stdout: stdout, Stderr: stderr,
		Getwd: os.Getwd, Now: time.Now,
		Interactive: isTerminal(stdin) && isTerminal(stderr),
		BinaryPath:  binaryPath, Version: version,
	}
	return cli.Execute(context.Background(), args, runtime)
}

func isTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}
