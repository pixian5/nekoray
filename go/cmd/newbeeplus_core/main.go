package main

import (
	"fmt"
	"os"
	_ "unsafe"

	"grpc_server"

	"github.com/matsuridayo/libneko/neko_common"
)

const runModeGuiCore = neko_common.RunMode_NekoRay_Core + 1

func main() {
	lines := make([]string, 0, 2)

	displayVersion := "not found"
	if externalBinary, err := resolveSingBoxExecutablePath(); err == nil {
		if externalVersion := readSingBoxVersionFromBinary(externalBinary); externalVersion != "" {
			displayVersion = externalVersion
		} else {
			displayVersion = "unknown"
		}
	}
	lines = append(lines, "sing-box: "+displayVersion)

	if xrayBinary, err := resolveXrayExecutablePath(); err == nil {
		if xrayVersion := readXrayVersionFromBinary(xrayBinary); xrayVersion != "" {
			lines = append(lines, "xray: "+xrayVersion)
		}
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	fmt.Println()

	// GUI core mode
	if len(os.Args) > 1 && os.Args[1] == "newbeeplus" {
		neko_common.RunMode = runModeGuiCore
		grpc_server.RunCore(setupCore, &server{})
		return
	}

	// sing-box CLI
	if handled, code := runExternalSingBoxCLI(os.Args[1:]); handled {
		os.Exit(code)
	}

	fmt.Fprintln(os.Stderr, "Error: external sing-box executable not found")
	os.Exit(1)
}
