package main

import (
	"github.com/giovantenne/nixorium/internal/npmartifact"
	"os"
)

func main() {
	if len(os.Args) != 4 || npmartifact.RunResolver(os.Args[1], os.Args[2], os.Args[3]) != nil {
		os.Exit(1)
	}
}
