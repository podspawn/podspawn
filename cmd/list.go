package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List active sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := state.Open(cfg.State.DBPath)
		if err != nil {
			return fmt.Errorf("opening state db: %w", err)
		}
		defer func() { _ = store.Close() }()

		sessions, err := store.ListSessions()
		if err != nil {
			return fmt.Errorf("listing sessions: %w", err)
		}

		if len(sessions) == 0 {
			fmt.Println("No active sessions.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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
		return w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
