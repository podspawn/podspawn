package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/podspawn/podspawn/internal/ui"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List machines",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := state.Open(cfg.State.DBPath)
		if err != nil {
			return fmt.Errorf("opening state db: %w", err)
		}
		defer func() { _ = store.Close() }()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if isLocalMode {
			machinesOnly, _ := cmd.Flags().GetBool("machines")
			rows, err := collectLocalMachineRows(store, os.Getenv("USER"), machinesOnly)
			if err != nil {
				return fmt.Errorf("listing machines: %w", err)
			}
			if len(rows) == 0 {
				fmt.Println("No machines.")
				return nil
			}

			_, _ = fmt.Fprintln(w, ui.Bold("NAME")+"\t"+ui.Bold("STATUS")+"\t"+ui.Bold("BRANCH")+"\t"+ui.Bold("IMAGE")+"\t"+ui.Bold("AGE"))
			for _, row := range rows {
				branch := row.Branch
				if branch == "" {
					branch = "-"
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					row.Name, ui.ColorStatus(row.Status), branch, ui.Faint(row.Image), row.Age,
				)
			}
		} else {
			sessions, err := store.ListSessions()
			if err != nil {
				return fmt.Errorf("listing machines: %w", err)
			}
			if len(sessions) == 0 {
				fmt.Println("No machines running.")
				return nil
			}

			_, _ = fmt.Fprintln(w, "USER\tPROJECT\tCONTAINER\tSTATUS\tCONNS\tAGE\tLIFETIME LEFT")
			for _, sess := range sessions {
				project := sess.Project
				if project == "" {
					project = "(default)"
				}
				age := cleanup.FormatDuration(time.Since(sess.CreatedAt))
				remaining := time.Until(sess.MaxLifetime)
				lifetime := cleanup.FormatDuration(remaining)
				if remaining <= 0 {
					lifetime = "expired"
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
					sess.User, project, sess.ContainerName,
					sess.Status, sess.Connections, age, lifetime,
				)
			}
		}
		return w.Flush()
	},
}

type localMachineStore interface {
	ListSessionsByUser(user string) ([]*state.Session, error)
	ListWorkspacesByUser(user string) ([]*state.Workspace, error)
}

type localMachineRow struct {
	Name   string
	Status string
	Branch string
	Image  string
	Age    string
}

func collectLocalMachineRows(store localMachineStore, user string, machinesOnly bool) ([]localMachineRow, error) {
	sessions, err := store.ListSessionsByUser(user)
	if err != nil {
		return nil, err
	}
	machines, err := store.ListWorkspacesByUser(user)
	if err != nil {
		return nil, err
	}

	return collectMachineRows(machines, sessions, machinesOnly), nil
}

func collectRegisteredMachineRows(store localMachineStore, user string) ([]localMachineRow, error) {
	sessions, err := store.ListSessionsByUser(user)
	if err != nil {
		return nil, err
	}
	machines, err := store.ListWorkspacesByUser(user)
	if err != nil {
		return nil, err
	}

	return collectMachineRows(machines, sessions, true), nil
}

func collectMachineRows(machines []*state.Workspace, sessions []*state.Session, machinesOnly bool) []localMachineRow {
	rowsByName := make(map[string]localMachineRow, len(sessions)+len(machines))
	for _, machine := range machines {
		status := "stopped"
		if !machine.Initialized {
			status = "uninitialized"
		}
		if machine.State == state.WorkspaceStatePreserved {
			status = "preserved"
		}
		rowsByName[machine.Name] = localMachineRow{
			Name:   machine.Name,
			Status: status,
			Branch: machine.Branch,
			Age:    cleanup.FormatDuration(time.Since(machine.CreatedAt)),
		}
	}

	for _, sess := range sessions {
		name := sess.Project
		if name == "" {
			name = "(default)"
		}
		existing, hasWorkspace := rowsByName[name]
		if machinesOnly && !hasWorkspace {
			continue
		}
		// Preserved workspaces stay preserved in the listing even if a
		// stale session row briefly coexists (e.g. between workspace
		// state update and session cleanup on a fatal on_create).
		if hasWorkspace && existing.Status == "preserved" {
			continue
		}

		status := sess.Status
		if status == state.StatusGracePeriod {
			status = "grace"
		}

		rowsByName[name] = localMachineRow{
			Name:   name,
			Status: status,
			Branch: existing.Branch,
			Image:  sess.Image,
			Age:    cleanup.FormatDuration(time.Since(sess.CreatedAt)),
		}
	}

	names := make([]string, 0, len(rowsByName))
	for name := range rowsByName {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([]localMachineRow, 0, len(names))
	for _, name := range names {
		rows = append(rows, rowsByName[name])
	}
	return rows
}

func init() {
	listCmd.Flags().Bool("machines", false, "show registered machines only in local mode")
	rootCmd.AddCommand(listCmd)
}
