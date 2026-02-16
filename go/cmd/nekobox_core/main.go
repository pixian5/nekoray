package main

import (
	"fmt"
	"os"
	_ "unsafe"

	"grpc_server"

	"github.com/matsuridayo/libneko/neko_common"
	boxmain "github.com/sagernet/sing-box/cmd/sing-box"
	"github.com/sagernet/sing-box/constant"
)

func main() {
	lines := make([]string, 0, 2)

	displayVersion := constant.Version
	if externalBinary, err := resolveSingBoxExecutablePath(); err == nil {
		if externalVersion := readSingBoxVersionFromBinary(externalBinary); externalVersion != "" {
			displayVersion = externalVersion
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
		neko_common.RunMode = neko_common.RunMode_NekoBox_Core
		grpc_server.RunCore(setupCore, &server{})
		return
	}

	// sing-box CLI
	if handled, code := runExternalSingBoxCLI(os.Args[1:]); handled {
		os.Exit(code)
	}

	// fallback: embedded sing-box
	boxmain.Main()
}
