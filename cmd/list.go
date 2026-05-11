package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/session"
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

			svc := session.New(session.Options{
				SessionStore:   store,
				WorkspaceStore: store,
			})
			res, err := svc.List(context.Background(), session.ListRequest{
				User:           os.Getenv("USER"),
				RegisteredOnly: machinesOnly,
			})
			if err != nil {
				return fmt.Errorf("listing machines: %w", err)
			}
			if len(res.Rows) == 0 {
				fmt.Println("No machines.")
				return nil
			}

			_, _ = fmt.Fprintln(w, ui.Bold("NAME")+"\t"+ui.Bold("STATUS")+"\t"+ui.Bold("BRANCH")+"\t"+ui.Bold("IMAGE")+"\t"+ui.Bold("AGE"))
			for _, row := range res.Rows {
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

func init() {
	listCmd.Flags().Bool("machines", false, "show registered machines only in local mode")
	rootCmd.AddCommand(listCmd)
}
