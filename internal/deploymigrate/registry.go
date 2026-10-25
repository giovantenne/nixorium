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
				}, Later: []string{
					// 2026-10-screensaver-60-fps applied, as released in 3.10.0.
					"29f1b0ff986cc02d91db8f39dc48531b1d48c08ef5788548be7d19d352b395f6",
				}},
				{Path: "scripts/screensaver-monitor.sh", Content: frozen(id, "screensaver-monitor.sh"), Released: []string{
					"1ca0eda66faa35f0cc974da0bc92b97d1a4cd9d1081901905b1a0f3a0a3ac128",
					"9a8d5fd89b6ddd475d6f0bfb5b767cee9fd7ba67b1ab6d0b92b58d09ea954359",
				}},
			})
		},
	},
	{
		ID:          "2026-10-screensaver-60-fps",
		Description: "The lab screensaver draws 60 frames per second instead of 30: its effects advance once per frame, so at 30 they ran at half speed.",
		Apply: func(tree Tree) Outcome {
			return ReplaceExact(tree, "scripts/cmd-screensaver.sh",
				"  # 30 frames per second look the same here and cost the terminal a quarter of\n  # the drawing that 120 would.\n  ttfx -i \"$SCREENSAVER_TEXT\" \\\n    --frame-rate 30 \\\n",
				"  # Effects advance once per frame, so the frame rate is also their speed: 60\n  # keeps them lively for half the drawing that 120 cost the terminal.\n  ttfx -i \"$SCREENSAVER_TEXT\" \\\n    --frame-rate 60 \\\n")
		},
	},
	{
		ID:          "2026-10-teacher-dock-classroom-launcher",
		Description: "The teacher's dock icon is always the classroom launcher, which opens the Nixorium menu when the classroom view is off.",
		Apply: func(tree Tree) Outcome {
			return ReplaceExact(tree, "modules/workstation.nix",
				"  teacherFavorites = lib.optionals isController\n    [ (if labSettings.classroomView or true then \"nixorium-classroom.desktop\" else \"nixorium.desktop\") ]\n",
				"  teacherFavorites = lib.optionals isController\n    [ \"nixorium-classroom.desktop\" ]\n")
		},
	},
}
