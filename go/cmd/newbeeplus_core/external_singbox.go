package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	envSingBoxMode = "NEKOBOX_SINGBOX_MODE"
	envSingBoxPath = "NEKOBOX_SINGBOX_PATH"
)

func getSingBoxMode() string {
	mode := strings.TrimSpace(strings.ToLower(os.Getenv(envSingBoxMode)))
	if mode == "" {
		return "external-prefer"
	}
	return mode
}

func singBoxExternalPreferred() bool {
	return getSingBoxMode() != "embedded"
}

func singBoxExternalRequired() bool {
	mode := getSingBoxMode()
	return mode == "external" || mode == "external-only"
}

func singBoxExecutableName() string {
	if runtime.GOOS == "windows" {
		return "sing-box.exe"
	}
	return "sing-box"
}

func resolvePathAgainstExecutable(path string) string {
	if path == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return path
	}
	exe, err := os.Executable()
	if err != nil {
		return path
	}
	return filepath.Clean(filepath.Join(filepath.Dir(exe), path))
}

func resolveSingBoxExecutablePath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(envSingBoxPath)); configured != "" {
		candidate := resolvePathAgainstExecutable(configured)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, nil
		}
	}

	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(exeDir, "core", singBoxExecutableName()),
			filepath.Join(exeDir, singBoxExecutableName()),
		}
		for _, candidate := range candidates {
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate, nil
			}
		}
	}

	if fromPath, err := exec.LookPath("sing-box"); err == nil {
		return fromPath, nil
	}

	return "", errors.New("sing-box executable not found")
}

func parseSingBoxVersion(output string) string {
	re := regexp.MustCompile(`(?i)sing-box version\s+([^\s]+)`)
	m := re.FindStringSubmatch(output)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func readSingBoxVersionFromBinary(binary string) string {
	out, err := exec.Command(binary, "version").CombinedOutput()
	if err != nil {
		return ""
	}
	return parseSingBoxVersion(string(out))
}

func runExternalSingBoxCLI(args []string) (handled bool, exitCode int) {
	binary, err := resolveSingBoxExecutablePath()
	if err != nil {
		return false, 0
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return true, ee.ExitCode()
		}
		return true, 1
	}
	return true, 0
}
