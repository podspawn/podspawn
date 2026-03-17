package cmd

import "testing"

func TestCreateCmdUsage(t *testing.T) {
	if createCmd.Use != "create <name>" {
		t.Errorf("create Use = %q, want 'create <name>'", createCmd.Use)
	}
	if createCmd.Args == nil {
		t.Error("create should require exactly 1 arg")
	}
}

func TestCreateCmdHasImageFlag(t *testing.T) {
	flag := createCmd.Flags().Lookup("image")
	if flag == nil {
		t.Fatal("create should have --image flag")
	}
	if flag.DefValue != "" {
		t.Errorf("--image default = %q, want empty", flag.DefValue)
	}
}
