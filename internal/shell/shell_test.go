package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInitScriptChangesParentShellDirectoryAndRecordsVisit(t *testing.T) {
	requirePOSIXShell(t)
	for _, shellName := range []string{"zsh", "bash", "fish"} {
		t.Run(shellName, func(t *testing.T) {
			shellPath, err := exec.LookPath(shellName)
			if err != nil {
				t.Skipf("%s unavailable", shellName)
			}
			root := t.TempDir()
			target := filepath.Join(root, "space dir")
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatal(err)
			}
			logFile := filepath.Join(root, "calls.log")
			binary := filepath.Join(root, "fake-jd")
			fake := "#!/bin/sh\n" +
				"if [ \"$1\" = \"_record\" ]; then printf 'record:%s\\n' \"$2\" >> \"" + logFile + "\"; exit 0; fi\n" +
				"printf '%s\\n' \"" + target + "\"\n"
			if err := os.WriteFile(binary, []byte(fake), 0o755); err != nil {
				t.Fatal(err)
			}
			initScript, err := Init(Options{Shell: shellName, Binary: binary, Bind: "jd", TrackShellCD: false})
			if err != nil {
				t.Fatal(err)
			}
			script := initScript + "\njd project\nprintf 'pwd:%s\\n' \"$PWD\"\n"
			args := []string{"-c", script}
			if shellName == "zsh" {
				args = []string{"-dfc", script}
			} else if shellName == "bash" {
				args = []string{"--noprofile", "--norc", "-c", script}
			} else {
				args = []string{"--no-config", "-c", script}
			}
			output, err := exec.Command(shellPath, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("%s integration failed: %v\n%s", shellName, err, output)
			}
			if string(output) != "pwd:"+target+"\n" {
				t.Fatalf("%s output=%q", shellName, output)
			}
			logData, err := os.ReadFile(logFile)
			if err != nil {
				t.Fatal(err)
			}
			if string(logData) != "record:"+target+"\n" {
				t.Fatalf("%s record log=%q", shellName, logData)
			}
		})
	}
}

func TestFishManagementCommandDelegatesWithoutChangingDirectory(t *testing.T) {
	requirePOSIXShell(t)
	shellPath, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish unavailable")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "fake-jd")
	fake := "#!/bin/sh\nprintf 'args:%s\\n' \"$*\"\n"
	if err := os.WriteFile(binary, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	initScript, err := Init(Options{Shell: "fish", Binary: binary, Bind: "jd", TrackShellCD: false})
	if err != nil {
		t.Fatal(err)
	}
	script := "cd \"" + root + "\"\n" + initScript + "\njd pin work\nprintf 'pwd:%s\\n' \"$PWD\"\n"
	output, err := exec.Command(shellPath, "--no-config", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("fish integration failed: %v\n%s", err, output)
	}
	if string(output) != "args:pin work\npwd:"+root+"\n" {
		t.Fatalf("management output=%q", output)
	}
}

func TestManagementCommandDelegatesWithoutChangingDirectory(t *testing.T) {
	requirePOSIXShell(t)
	shellPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh unavailable")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "fake-jd")
	fake := "#!/bin/sh\nprintf 'args:%s\\n' \"$*\"\n"
	if err := os.WriteFile(binary, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	initScript, err := Init(Options{Shell: "zsh", Binary: binary, Bind: "jd", TrackShellCD: false})
	if err != nil {
		t.Fatal(err)
	}
	script := "cd \"" + root + "\"\n" + initScript + "\njd pin work\nprintf 'pwd:%s\\n' \"$PWD\"\n"
	output, err := exec.Command(shellPath, "-dfc", script).CombinedOutput()
	if err != nil {
		t.Fatalf("zsh integration failed: %v\n%s", err, output)
	}
	if string(output) != "args:pin work\npwd:"+root+"\n" {
		t.Fatalf("management output=%q", output)
	}
}

func TestHelpFlagDelegatesWithoutChangingDirectory(t *testing.T) {
	requirePOSIXShell(t)
	shellPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh unavailable")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "fake-jd")
	fake := "#!/bin/sh\nprintf 'help text\\n'\n"
	if err := os.WriteFile(binary, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	initScript, err := Init(Options{Shell: "zsh", Binary: binary, Bind: "jd", TrackShellCD: false})
	if err != nil {
		t.Fatal(err)
	}
	script := "cd \"" + root + "\"\n" + initScript + "\njd jump --help\nprintf 'pwd:%s\\n' \"$PWD\"\n"
	output, err := exec.Command(shellPath, "-dfc", script).CombinedOutput()
	if err != nil {
		t.Fatalf("zsh integration failed: %v\n%s", err, output)
	}
	if string(output) != "help text\npwd:"+root+"\n" {
		t.Fatalf("help output=%q", output)
	}
}

func TestEndOfOptionsAllowsHelpNamedDirectory(t *testing.T) {
	requirePOSIXShell(t)
	for _, shellName := range []string{"zsh", "bash", "fish"} {
		t.Run(shellName, func(t *testing.T) {
			shellPath, err := exec.LookPath(shellName)
			if err != nil {
				t.Skipf("%s unavailable", shellName)
			}
			root := t.TempDir()
			target := filepath.Join(root, "--help")
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(root, "fake-jd")
			fake := "#!/bin/sh\nprintf '%s\\n' \"" + target + "\"\n"
			if err := os.WriteFile(binary, []byte(fake), 0o755); err != nil {
				t.Fatal(err)
			}
			initScript, err := Init(Options{Shell: shellName, Binary: binary, Bind: "jd", TrackShellCD: false})
			if err != nil {
				t.Fatal(err)
			}
			script := "cd \"" + root + "\"\n" + initScript + "\njd -- --help\nprintf 'pwd:%s\\n' \"$PWD\"\n"
			args := []string{"-c", script}
			switch shellName {
			case "zsh":
				args = []string{"-dfc", script}
			case "bash":
				args = []string{"--noprofile", "--norc", "-c", script}
			case "fish":
				args = []string{"--no-config", "-c", script}
			}
			output, err := exec.Command(shellPath, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("%s integration failed: %v\n%s", shellName, err, output)
			}
			if string(output) != "pwd:"+target+"\n" {
				t.Fatalf("%s end-of-options output=%q", shellName, output)
			}
		})
	}
}

func TestVersionFlagDelegatesWithoutChangingDirectory(t *testing.T) {
	requirePOSIXShell(t)
	shellPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh unavailable")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "fake-jd")
	fake := "#!/bin/sh\nprintf 'jd 0.1.0\\n'\n"
	if err := os.WriteFile(binary, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	initScript, err := Init(Options{Shell: "zsh", Binary: binary, Bind: "jd", TrackShellCD: false})
	if err != nil {
		t.Fatal(err)
	}
	script := "cd \"" + root + "\"\n" + initScript + "\njd --version\nprintf 'pwd:%s\\n' \"$PWD\"\n"
	output, err := exec.Command(shellPath, "-dfc", script).CombinedOutput()
	if err != nil {
		t.Fatalf("zsh integration failed: %v\n%s", err, output)
	}
	if string(output) != "jd 0.1.0\npwd:"+root+"\n" {
		t.Fatalf("version output=%q", output)
	}
}

func TestInitRejectsUnsafeBindingName(t *testing.T) {
	_, err := Init(Options{Shell: "zsh", Binary: "/tmp/jd", Bind: "bad;name"})
	if err == nil || !strings.Contains(err.Error(), "binding") {
		t.Fatalf("Init() error=%v; want invalid binding error", err)
	}
}

func TestPowerShellInitChangesDirectoryAndRecordsVisit(t *testing.T) {
	powerShell := "pwsh"
	if runtime.GOOS == "windows" {
		powerShell = "powershell.exe"
	}
	shellPath, err := exec.LookPath(powerShell)
	if err != nil {
		t.Skipf("%s unavailable", powerShell)
	}
	root := t.TempDir()
	target := filepath.Join(root, "space dir")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(root, "calls.log")
	binary := filepath.Join(root, "fake-jd")
	var fake string
	if runtime.GOOS == "windows" {
		binary += ".cmd"
		fake = "@echo off\r\n" +
			"if \"%1\"==\"_record\" (\r\n" +
			">>\"" + logFile + "\" echo record:%~2\r\n" +
			"exit /b 0\r\n" +
			")\r\n" +
			"if \"%1\"==\"--version\" (\r\n" +
			"echo jd 0.1.0\r\n" +
			"exit /b 0\r\n" +
			")\r\n" +
			"echo " + target + "\r\n"
	} else {
		fake = "#!/bin/sh\nif [ \"$1\" = \"_record\" ]; then printf 'record:%s\\n' \"$2\" >> \"" + logFile + "\"; exit 0; fi\nif [ \"$1\" = \"--version\" ]; then printf 'jd 0.1.0\\n'; exit 0; fi\nprintf '%s\\n' \"" + target + "\"\n"
	}
	if err := os.WriteFile(binary, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	initScript, err := Init(Options{Shell: "powershell", Binary: binary, Bind: "jd", TrackShellCD: false})
	if err != nil {
		t.Fatal(err)
	}
	script := initScript + "\njd project\njd '--' --help\njd --version\nWrite-Output ('pwd:' + $PWD.Path)\n"
	output, err := exec.Command(shellPath, "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell integration failed: %v\n%s", err, output)
	}
	outputLines := shellOutputLines(string(output))
	if len(outputLines) != 2 || outputLines[0] != "jd 0.1.0" || !strings.HasPrefix(outputLines[1], "pwd:") {
		t.Fatalf("PowerShell output=%q", output)
	}
	assertSameDirectory(t, strings.TrimPrefix(outputLines[1], "pwd:"), target)
	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	logLines := shellOutputLines(string(logData))
	if len(logLines) != 2 {
		t.Fatalf("PowerShell record log=%q", logData)
	}
	for _, line := range logLines {
		if !strings.HasPrefix(line, "record:") {
			t.Fatalf("PowerShell record log=%q", logData)
		}
		assertSameDirectory(t, strings.TrimPrefix(line, "record:"), target)
	}
}

func shellOutputLines(output string) []string {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.TrimSuffix(output, "\n")
	if output == "" {
		return nil
	}
	return strings.Split(output, "\n")
}

func assertSameDirectory(t *testing.T, got, want string) {
	t.Helper()
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat directory %q: %v", got, err)
	}
	wantInfo, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat directory %q: %v", want, err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("directory=%q; want same directory as %q", got, want)
	}
}

func requirePOSIXShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell integration test")
	}
}
