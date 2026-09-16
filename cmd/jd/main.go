package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/term"
	"jd/internal/cli"
	"jd/internal/config"
	"jd/internal/store"
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
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		fmt.Fprintf(stderr, "jd: load config: %v\n", err)
		return cli.ExitRuntime
	}
	database, err := store.Open(paths.DatabaseFile)
	if err != nil {
		fmt.Fprintf(stderr, "jd: open database %s: %v\n", paths.DatabaseFile, err)
		if len(args) > 0 && args[0] == "doctor" {
			fmt.Fprintln(stderr, "jd: back up or move this file aside, then run jd doctor again; jd will not delete it automatically")
		}
		return cli.ExitRuntime
	}
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
