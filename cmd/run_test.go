package cmd

import "testing"

func TestRunCmdUsage(t *testing.T) {
	if runCmd.Use != "run <name>" {
		t.Errorf("run Use = %q, want 'run <name>'", runCmd.Use)
	}
	if runCmd.Args == nil {
		t.Error("run should require exactly 1 arg")
	}
}

func TestRunCmdHasImageFlag(t *testing.T) {
	flag := runCmd.Flags().Lookup("image")
	if flag == nil {
		t.Fatal("run should have --image flag")
	}
}
