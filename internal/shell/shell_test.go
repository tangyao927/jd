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
		fake = "@echo off\r\nif \"%1\"==\"_record\" (echo record:%~2>>\"" + logFile + "\" & exit /b 0)\r\necho " + target + "\r\n"
	} else {
		fake = "#!/bin/sh\nif [ \"$1\" = \"_record\" ]; then printf 'record:%s\\n' \"$2\" >> \"" + logFile + "\"; exit 0; fi\nprintf '%s\\n' \"" + target + "\"\n"
	}
	if err := os.WriteFile(binary, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	initScript, err := Init(Options{Shell: "powershell", Binary: binary, Bind: "jd", TrackShellCD: false})
	if err != nil {
		t.Fatal(err)
	}
	script := initScript + "\njd project\nWrite-Output ('pwd:' + $PWD.Path)\n"
	output, err := exec.Command(shellPath, "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell integration failed: %v\n%s", err, output)
	}
	if strings.TrimSpace(string(output)) != "pwd:"+target {
		t.Fatalf("PowerShell output=%q", output)
	}
	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(logData)) != "record:"+target {
		t.Fatalf("PowerShell record log=%q", logData)
	}
}
