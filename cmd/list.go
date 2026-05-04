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

			_, _ = fmt.Fprintln(w, ui.Bold("NAME")+"\t"+ui.Bold("STATUS")+"\t"+ui.Bold("IMAGE")+"\t"+ui.Bold("AGE"))
			for _, row := range rows {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					row.Name, ui.ColorStatus(row.Status), ui.Faint(row.Image), row.Age,
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
	ListMachinesByUser(user string) ([]*state.Machine, error)
}

type localMachineRow struct {
	Name   string
	Status string
	Image  string
	Age    string
}

func collectLocalMachineRows(store localMachineStore, user string, machinesOnly bool) ([]localMachineRow, error) {
	sessions, err := store.ListSessionsByUser(user)
	if err != nil {
		return nil, err
	}
	machines, err := store.ListMachinesByUser(user)
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
	machines, err := store.ListMachinesByUser(user)
	if err != nil {
		return nil, err
	}

	return collectMachineRows(machines, sessions, true), nil
}

func collectMachineRows(machines []*state.Machine, sessions []*state.Session, machinesOnly bool) []localMachineRow {
	rowsByName := make(map[string]localMachineRow, len(sessions)+len(machines))
	for _, machine := range machines {
		status := "stopped"
		if !machine.Initialized {
			status = "uninitialized"
		}
		rowsByName[machine.Name] = localMachineRow{
			Name:   machine.Name,
			Status: status,
			Age:    cleanup.FormatDuration(time.Since(machine.CreatedAt)),
		}
	}

	for _, sess := range sessions {
		name := sess.Project
		if name == "" {
			name = "(default)"
		}
		if machinesOnly {
			if _, ok := rowsByName[name]; !ok {
				continue
			}
		}

		status := sess.Status
		if status == state.StatusGracePeriod {
			status = "grace"
		}

		rowsByName[name] = localMachineRow{
			Name:   name,
			Status: status,
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
