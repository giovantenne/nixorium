package deploymigrate

// Registry is the list of migrations of this release, in the order they
// run. Append new migrations at the end; never edit or remove a released one.
var Registry = []Migration{
	{
		ID:          "2026-10-workstation-is-controller",
		Description: "The workstation profile finds the controller with isController: computers that share a configuration have no single name (ADR 0033).",
		Apply: func(tree Tree) Outcome {
			return ReplaceExact(tree, "modules/workstation.nix",
				"  isController = args ? hostName && args.hostName == (labSettings.masterHostName or null);\n",
				"  isController = args.isController or false;\n")
		},
	},
	{
		ID:          "2026-10-jq-in-every-laboratory",
		Description: "jq joins the software every laboratory starts with, on the controller and all clients.",
		Apply: func(tree Tree) Outcome {
			return AddSharedPackage(tree, "jq")
		},
	},
	{
		ID:          "2026-10-ttfx-screensaver",
		Description: "The lab screensaver uses ttfx, the faster Rust version of its text effects, at 30 frames per second, and stops when the screen goes blank after 30 idle minutes.",
		Apply: func(tree Tree) Outcome {
			// Released forms: 3.6.0, and 3.6.1 up to 3.10.0-beta.3.
			const id = "2026-10-ttfx-screensaver"
			return ReplaceWholeFiles(tree, []WholeFile{
				{Path: "modules/screensaver.nix", Content: frozen(id, "screensaver.nix"), Released: []string{
					"5080f60ffc5dfe4244ea197886b6b0dab8d3199957d475d5b131c2ced14ba9ca",
					"a65abcd56b3624cd8159bad89c99bc9bd993e9f7e350d818e95281efc8cf3b4c",
				}},
				{Path: "scripts/cmd-screensaver.sh", Content: frozen(id, "cmd-screensaver.sh"), Released: []string{
					"c4bc7fb93494dd55f2e4a1209032abb2783d8d69764a83d4697f5c52da5dcc8d",
				}},
				{Path: "scripts/screensaver-monitor.sh", Content: frozen(id, "screensaver-monitor.sh"), Released: []string{
					"1ca0eda66faa35f0cc974da0bc92b97d1a4cd9d1081901905b1a0f3a0a3ac128",
					"9a8d5fd89b6ddd475d6f0bfb5b767cee9fd7ba67b1ab6d0b92b58d09ea954359",
				}},
			})
		},
	},
}
