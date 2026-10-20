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
}
