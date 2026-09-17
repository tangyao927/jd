package install_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
		command := exec.Command("sh", script, "--binary", fake, "--shell", "zsh", "--bind", "jd_test")
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
	command := exec.Command("sh", filepath.Join(projectRoot(t), "scripts", "install.sh"), "--dry-run", "--binary", fake, "--shell", "zsh", "--bind", "jd_test")
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

func TestUnixInstallerDownloadsAndVerifiesRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer test")
	}
	root := t.TempDir()
	assetName := fmt.Sprintf("jd_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	binary := []byte("#!/bin/sh\ncase \"$1\" in init) printf 'jd_test() { :; }\\n' ;; completion) printf '# completion\\n' ;; esac\n")
	releaseRoot := filepath.Join(root, "releases")
	writeUnixRelease(t, filepath.Join(releaseRoot, "download", "v0.1.0"), assetName, binary, false)

	binDir := filepath.Join(root, "bin")
	profile := filepath.Join(root, "zshrc")
	command := exec.Command("sh", filepath.Join(projectRoot(t), "scripts", "install.sh"), "--version", "v0.1.0", "--shell", "zsh", "--bind", "jd_test")
	command.Env = append(os.Environ(),
		"JD_RELEASE_BASE_URL=file://"+releaseRoot,
		"JD_INSTALL_BIN_DIR="+binDir,
		"JD_INSTALL_PROFILE="+profile,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("release install failed: %v\n%s", err, output)
	}
	installed, err := os.ReadFile(filepath.Join(binDir, "jd"))
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(binary) {
		t.Fatalf("installed binary=%q; want release binary", installed)
	}
}

func TestUnixInstallerRejectsChecksumMismatchWithoutReplacingBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer test")
	}
	root := t.TempDir()
	assetName := fmt.Sprintf("jd_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	releaseRoot := filepath.Join(root, "releases")
	writeUnixRelease(t, filepath.Join(releaseRoot, "download", "v0.1.0"), assetName, []byte("new binary"), true)

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(binDir, "jd")
	if err := os.WriteFile(destination, []byte("existing binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", filepath.Join(projectRoot(t), "scripts", "install.sh"), "--version", "v0.1.0", "--shell", "zsh", "--bind", "jd_test")
	command.Env = append(os.Environ(),
		"JD_RELEASE_BASE_URL=file://"+releaseRoot,
		"JD_INSTALL_BIN_DIR="+binDir,
		"JD_INSTALL_PROFILE="+filepath.Join(root, "zshrc"),
	)
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("checksum mismatch succeeded:\n%s", output)
	}
	installed, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "existing binary" {
		t.Fatalf("checksum failure replaced destination with %q", installed)
	}
}

func TestUnixReleaseDryRunDoesNotAccessNetwork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer test")
	}
	root := t.TempDir()
	command := exec.Command("sh", filepath.Join(projectRoot(t), "scripts", "install.sh"), "--dry-run", "--version", "v0.1.0", "--shell", "zsh", "--bind", "jd_test")
	command.Env = append(os.Environ(),
		"JD_RELEASE_BASE_URL=http://127.0.0.1:1",
		"JD_INSTALL_BIN_DIR="+filepath.Join(root, "bin"),
		"JD_INSTALL_PROFILE="+filepath.Join(root, "zshrc"),
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("release dry run failed or accessed network: %v\n%s", err, output)
	}
}

func TestUnixInstallerRejectsInvalidReleaseVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer test")
	}
	root := t.TempDir()
	command := exec.Command("sh", filepath.Join(projectRoot(t), "scripts", "install.sh"), "--dry-run", "--version", "v1.2.3-beta", "--shell", "zsh", "--bind", "jd_test")
	command.Env = append(os.Environ(),
		"JD_INSTALL_BIN_DIR="+filepath.Join(root, "bin"),
		"JD_INSTALL_PROFILE="+filepath.Join(root, "zshrc"),
	)
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("invalid release version succeeded:\n%s", output)
	}
}

func writeUnixRelease(t *testing.T, directory, assetName string, binary []byte, corruptChecksum bool) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	assetPath := filepath.Join(directory, assetName)
	file, err := os.Create(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	multi := io.MultiWriter(file, hash)
	gzipWriter := gzip.NewWriter(multi)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "jd", Mode: 0o755, Size: int64(len(binary))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", hash.Sum(nil))
	if corruptChecksum {
		digest = strings.Repeat("0", 64)
	}
	if err := os.WriteFile(filepath.Join(directory, "checksums.txt"), []byte(digest+"  "+assetName+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPowerShellInstallerDownloadsAndVerifiesRelease(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows installer test")
	}
	root := t.TempDir()
	assetName := fmt.Sprintf("jd_windows_%s.zip", runtime.GOARCH)
	binary := []byte("release-binary")
	releaseRoot := filepath.Join(root, "releases")
	writeWindowsRelease(t, filepath.Join(releaseRoot, "download", "v0.1.0"), assetName, binary, false)
	server := httptest.NewServer(http.FileServer(http.Dir(releaseRoot)))
	t.Cleanup(server.Close)

	binDir := filepath.Join(root, "bin")
	profile := filepath.Join(root, "profile.ps1")
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(projectRoot(t), "scripts", "install.ps1"), "-Version", "v0.1.0", "-Bind", "jd_test")
	command.Env = append(os.Environ(),
		"JD_RELEASE_BASE_URL="+server.URL,
		"JD_INSTALL_BIN_DIR="+binDir,
		"JD_INSTALL_PROFILE="+profile,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell release install failed: %v\n%s", err, output)
	}
	installed, err := os.ReadFile(filepath.Join(binDir, "jd.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(binary) {
		t.Fatalf("installed binary=%q; want release binary", installed)
	}
}

func TestPowerShellInstallerRejectsChecksumMismatch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows installer test")
	}
	root := t.TempDir()
	assetName := fmt.Sprintf("jd_windows_%s.zip", runtime.GOARCH)
	releaseRoot := filepath.Join(root, "releases")
	writeWindowsRelease(t, filepath.Join(releaseRoot, "download", "v0.1.0"), assetName, []byte("new binary"), true)
	server := httptest.NewServer(http.FileServer(http.Dir(releaseRoot)))
	t.Cleanup(server.Close)

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(binDir, "jd.exe")
	if err := os.WriteFile(destination, []byte("existing binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(projectRoot(t), "scripts", "install.ps1"), "-Version", "v0.1.0", "-Bind", "jd_test")
	command.Env = append(os.Environ(),
		"JD_RELEASE_BASE_URL="+server.URL,
		"JD_INSTALL_BIN_DIR="+binDir,
		"JD_INSTALL_PROFILE="+filepath.Join(root, "profile.ps1"),
	)
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("PowerShell checksum mismatch succeeded:\n%s", output)
	} else if !strings.Contains(string(output), "Checksum mismatch") {
		t.Fatalf("PowerShell installer failed for the wrong reason: %v\n%s", err, output)
	}
	installed, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "existing binary" {
		t.Fatalf("checksum failure replaced destination with %q", installed)
	}
}

func TestPowerShellReleaseDryRunDoesNotAccessNetwork(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows installer test")
	}
	root := t.TempDir()
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(projectRoot(t), "scripts", "install.ps1"), "-DryRun", "-Version", "v0.1.0", "-Bind", "jd_test")
	command.Env = append(os.Environ(),
		"JD_RELEASE_BASE_URL=http://127.0.0.1:1",
		"JD_INSTALL_BIN_DIR="+filepath.Join(root, "bin"),
		"JD_INSTALL_PROFILE="+filepath.Join(root, "profile.ps1"),
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell release dry run failed or accessed network: %v\n%s", err, output)
	}
}

func writeWindowsRelease(t *testing.T, directory, assetName string, binary []byte, corruptChecksum bool) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	assetPath := filepath.Join(directory, assetName)
	file, err := os.Create(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	zipWriter := zip.NewWriter(io.MultiWriter(file, hash))
	entry, err := zipWriter.Create("jd.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", hash.Sum(nil))
	if corruptChecksum {
		digest = strings.Repeat("0", 64)
	}
	if err := os.WriteFile(filepath.Join(directory, "checksums.txt"), []byte(digest+"  "+assetName+"\n"), 0o644); err != nil {
		t.Fatal(err)
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
