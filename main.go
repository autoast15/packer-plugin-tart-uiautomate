package main

import (
	"fmt"
	"github.com/peter1122999/packer-plugin-tart-uiautomate/builder/tart"
	"github.com/peter1122999/packer-plugin-tart-uiautomate/version"
	"os"

	"github.com/hashicorp/packer-plugin-sdk/plugin"
)

func main() {
	pps := plugin.NewSet()
	pps.RegisterBuilder("cli", new(tart.Builder))
	pps.SetVersion(version.PluginVersion)
	err := pps.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
