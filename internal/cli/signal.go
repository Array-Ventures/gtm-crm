package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/Array-Ventures/gtm-crm/internal/db/repo"
	"github.com/Array-Ventures/gtm-crm/internal/format"
	"github.com/Array-Ventures/gtm-crm/internal/model"
	"github.com/spf13/cobra"
)

var signalColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Type", Field: "signal_type"},
	{Header: "Description", Field: "description"},
	{Header: "Person", Field: "person_id"},
	{Header: "Org", Field: "org_id"},
	{Header: "Detected", Field: "detected_at"},
}

func signalToMap(s *model.Signal) map[string]any {
	m := map[string]any{
		"id":          s.ID,
		"uuid":        s.UUID,
		"signal_type": s.SignalType,
		"detected_at": s.DetectedAt,
		"created_at":  s.CreatedAt,
		"updated_at":  s.UpdatedAt,
	}
	if s.Description != nil {
		m["description"] = *s.Description
	}
	if s.PersonID != nil {
		m["person_id"] = *s.PersonID
	}
	if s.OrgID != nil {
		m["org_id"] = *s.OrgID
	}
	return m
}

func signalsToMaps(signals []*model.Signal) []map[string]any {
	result := make([]map[string]any, len(signals))
	for i, s := range signals {
		result[i] = signalToMap(s)
	}
	return result
}

func registerSignalCommands(rootCmd *cobra.Command) {
	signalCmd := &cobra.Command{
		Use:   "signal",
		Short: "Manage go-to-market signals",
	}

	signalCmd.AddCommand(signalAddCmd())
	signalCmd.AddCommand(signalListCmd())
	signalCmd.AddCommand(signalShowCmd())
	signalCmd.AddCommand(signalDeleteCmd())

	rootCmd.AddCommand(signalCmd)
}

func signalAddCmd() *cobra.Command {
	var description, detectedAt string
	var personID, orgID int64

	cmd := &cobra.Command{
		Use:   "add <type>",
		Short: "Add a new signal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			input := model.CreateSignalInput{
				SignalType:  args[0],
				Description: nilIfEmpty(description),
				DetectedAt:  nilIfEmpty(detectedAt),
			}
			if cmd.Flags().Changed("person") {
				input.PersonID = &personID
			}
			if cmd.Flags().Changed("org") {
				input.OrgID = &orgID
			}

			r := repo.NewSignalRepo(db)
			signal, err := r.Create(cmd.Context(), input)
			if err != nil {
				return err
			}

			data := []map[string]any{signalToMap(signal)}
			return format.Output(os.Stdout, resolveFormat(), data, signalColumns, flagQuiet)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "what the signal is")
	cmd.Flags().Int64Var(&personID, "person", 0, "associated person ID")
	cmd.Flags().Int64Var(&orgID, "org", 0, "associated organization ID")
	cmd.Flags().StringVar(&detectedAt, "at", "", "when the signal was detected (ISO 8601)")

	return cmd
}

func signalListCmd() *cobra.Command {
	var signalType string
	var personID, orgID int64
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List signals",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			filters := model.SignalFilters{Limit: limit}
			if signalType != "" {
				filters.SignalType = &signalType
			}
			if cmd.Flags().Changed("person") {
				filters.PersonID = &personID
			}
			if cmd.Flags().Changed("org") {
				filters.OrgID = &orgID
			}

			r := repo.NewSignalRepo(db)
			signals, err := r.FindAll(cmd.Context(), filters)
			if err != nil {
				return err
			}

			return format.Output(os.Stdout, resolveFormat(), signalsToMaps(signals), signalColumns, flagQuiet)
		},
	}

	cmd.Flags().StringVar(&signalType, "type", "", "filter by signal type")
	cmd.Flags().Int64Var(&personID, "person", 0, "filter by person ID")
	cmd.Flags().Int64Var(&orgID, "org", 0, "filter by organization ID")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results")

	return cmd
}

func signalShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show signal details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return model.NewExitError(model.ErrValidation, "invalid signal ID: %s", args[0])
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			r := repo.NewSignalRepo(db)
			signal, err := r.FindByID(cmd.Context(), id)
			if err != nil {
				return err
			}

			data := []map[string]any{signalToMap(signal)}
			return format.Output(os.Stdout, resolveFormat(), data, signalColumns, flagQuiet)
		},
	}
}

func signalDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a signal (soft-delete)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return model.NewExitError(model.ErrValidation, "invalid signal ID: %s", args[0])
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			r := repo.NewSignalRepo(db)
			if err := r.Archive(cmd.Context(), id); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Deleted signal #%d\n", id)
			return nil
		},
	}
}
