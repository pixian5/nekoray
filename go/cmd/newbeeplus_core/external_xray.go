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

const envXrayPath = "NEKOBOX_XRAY_PATH"

func xrayExecutableName() string {
	if runtime.GOOS == "windows" {
		return "xray.exe"
	}
	return "xray"
}

func resolveXrayExecutablePath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(envXrayPath)); configured != "" {
		candidate := resolvePathAgainstExecutable(configured)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, nil
		}
	}

	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(exeDir, "core", xrayExecutableName()),
			filepath.Join(exeDir, "xray_core", xrayExecutableName()),
			filepath.Join(exeDir, xrayExecutableName()),
		}
		for _, candidate := range candidates {
			if st, statErr := os.Stat(candidate); statErr == nil && !st.IsDir() {
				return candidate, nil
			}
		}
	}

	if fromPath, err := exec.LookPath("xray"); err == nil {
		return fromPath, nil
	}

	return "", errors.New("xray executable not found")
}

func parseXrayVersion(output string) string {
	re := regexp.MustCompile(`(?i)xray\s+([^\s]+)`)
	m := re.FindStringSubmatch(output)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func readXrayVersionFromBinary(binary string) string {
	out, err := exec.Command(binary, "version").CombinedOutput()
	if err == nil {
		if version := parseXrayVersion(string(out)); version != "" {
			return version
		}
	}

	out, err = exec.Command(binary, "-version").CombinedOutput()
	if err != nil {
		return ""
	}
	return parseXrayVersion(string(out))
}
