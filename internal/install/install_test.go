package install_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUnixInstallerIsIdempotentAndUninstallPreservesData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer test")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	profile := filepath.Join(root, "zshrc")
	dataDir := filepath.Join(root, "data")
	if err := os.WriteFile(profile, []byte("export KEEP_ME=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dataFile := filepath.Join(dataDir, "state.db")
	if err := os.WriteFile(dataFile, []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(root, "source-jd")
	fakeScript := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  init) printf 'jd() { :; }\\n' ;;\n" +
		"  completion) printf 'compdef _jd jd\\n' ;;\n" +
		"esac\n"
	if err := os.WriteFile(fake, []byte(fakeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(projectRoot(t), "scripts", "install.sh")
	env := append(os.Environ(),
		"JD_INSTALL_BIN_DIR="+binDir,
		"JD_INSTALL_PROFILE="+profile,
		"JD_DATA_DIR="+dataDir,
	)
	for i := 0; i < 2; i++ {
		command := exec.Command("sh", script, "--binary", fake, "--shell", "zsh", "--bind", "jd")
		command.Env = env
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("install attempt %d failed: %v\n%s", i+1, err, output)
		}
	}
	profileData, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	profileText := string(profileData)
	if strings.Count(profileText, "# >>> jd initialize >>>") != 1 || !strings.Contains(profileText, "export KEEP_ME=1") {
		t.Fatalf("profile after reinstall:\n%s", profileText)
	}
	if _, err := os.Stat(filepath.Join(binDir, "jd")); err != nil {
		t.Fatalf("installed binary missing: %v", err)
	}
	if zsh, err := exec.LookPath("zsh"); err == nil {
		command := exec.Command(zsh, "-dfc", "source "+profile)
		if output, err := command.CombinedOutput(); err != nil || len(output) != 0 {
			t.Fatalf("installed profile is not safe in bare zsh: %v\n%s", err, output)
		}
	}

	command := exec.Command("sh", script, "--uninstall", "--shell", "zsh")
	command.Env = env
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, output)
	}
	profileData, err = os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(profileData), "jd initialize") || !strings.Contains(string(profileData), "export KEEP_ME=1") {
		t.Fatalf("profile after uninstall:\n%s", profileData)
	}
	if _, err := os.Stat(filepath.Join(binDir, "jd")); !os.IsNotExist(err) {
		t.Fatalf("binary remains after uninstall: %v", err)
	}
	if _, err := os.Stat(dataFile); err != nil {
		t.Fatalf("uninstall removed state: %v", err)
	}
}

func TestUnixInstallerDryRunDoesNotWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer test")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	profile := filepath.Join(root, "zshrc")
	fake := filepath.Join(root, "source-jd")
	if err := os.WriteFile(fake, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", filepath.Join(projectRoot(t), "scripts", "install.sh"), "--dry-run", "--binary", fake, "--shell", "zsh")
	command.Env = append(os.Environ(), "JD_INSTALL_BIN_DIR="+binDir, "JD_INSTALL_PROFILE="+profile)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("dry run failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("dry run created profile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "jd")); !os.IsNotExist(err) {
		t.Fatalf("dry run created binary: %v", err)
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
