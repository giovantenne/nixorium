package main

import "testing"

func TestTelemetryCommandParsing(t *testing.T) {
	for _, args := range [][]string{{"telemetry", "preview", "--json"}, {"telemetry", "status"}, {"telemetry", "enable"}, {"telemetry", "disable"}, {"telemetry", "send"}} {
		o, e := parseArguments(args)
		if e != nil || o.command != "telemetry" {
			t.Fatal(args, o, e)
		}
	}
	for _, args := range [][]string{{"telemetry"}, {"telemetry", "enable", "--yes"}, {"telemetry", "enable", "--json"}, {"telemetry", "status", "--repo", "/tmp"}, {"telemetry", "enable", "disable"}} {
		if _, e := parseArguments(args); e == nil {
			t.Fatal("accepted", args)
		}
	}
}
