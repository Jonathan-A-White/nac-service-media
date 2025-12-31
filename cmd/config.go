package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"nac-service-media/infrastructure/config"

	"github.com/spf13/cobra"
)

// DefaultOutput is the default output writer for config commands
var DefaultOutput OutputWriter = os.Stdout

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration entries",
	Long: `Manage ministers, recipients, CC recipients, and senders in the configuration file.

Examples:
  nac-service-media config list ministers
  nac-service-media config add minister --key smith --name "Rev. John Smith"
  nac-service-media config add sender --key avteam --name "A/V Team"
  nac-service-media config remove recipient jane`,
}

func init() {
	rootCmd.AddCommand(configCmd)

	// Add subcommands
	configCmd.AddCommand(configAddCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configRemoveCmd)
	configCmd.AddCommand(configUpdateCmd)
}

// --- ADD command ---

var (
	addKey   string
	addName  string
	addEmail string
)

var configAddCmd = &cobra.Command{
	Use:   "add [minister|recipient|cc|sender]",
	Short: "Add a new config entry",
	Long: `Add a new minister, recipient, CC, or sender to the configuration.

Examples:
  nac-service-media config add minister --key smith --name "Rev. John Smith"
  nac-service-media config add recipient --key jane --name "Jane Doe" --email "jane@example.com"
  nac-service-media config add cc --key mary --name "Mary Jones" --email "mary@example.com"
  nac-service-media config add sender --key avteam --name "White Plains NAC A/V Team"`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigAdd,
}

func init() {
	configAddCmd.Flags().StringVar(&addKey, "key", "", "Unique key for the entry (required)")
	configAddCmd.Flags().StringVar(&addName, "name", "", "Display name (required)")
	configAddCmd.Flags().StringVar(&addEmail, "email", "", "Email address (required for recipient and cc)")
	configAddCmd.MarkFlagRequired("key")
	configAddCmd.MarkFlagRequired("name")
}

func runConfigAdd(cmd *cobra.Command, args []string) error {
	cfg := GetConfig()
	if cfg == nil {
		return fmt.Errorf("config file not found. Run 'nac-service-media setup' first")
	}

	return RunConfigAddWithDependencies(cfg, cfgFile, args[0], addKey, addName, addEmail, DefaultOutput)
}

// RunConfigAddWithDependencies runs the add command with injected dependencies
func RunConfigAddWithDependencies(cfg *config.Config, configPath, entityType, key, name, email string, out OutputWriter) error {
	mgr := config.NewConfigManager(cfg, configPath)

	switch entityType {
	case "minister":
		if err := mgr.AddMinister(key, name); err != nil {
			return err
		}
		fmt.Fprintf(out, "Added minister %q: %s\n", key, name)

	case "recipient":
		if email == "" {
			return fmt.Errorf("--email is required for recipients")
		}
		if err := mgr.AddRecipient(key, name, email); err != nil {
			return err
		}
		fmt.Fprintf(out, "Added recipient %q: %s <%s>\n", key, name, email)

	case "cc":
		if email == "" {
			return fmt.Errorf("--email is required for cc")
		}
		if err := mgr.AddCC(key, name, email); err != nil {
			return err
		}
		fmt.Fprintf(out, "Added CC %q: %s <%s>\n", key, name, email)

	case "sender":
		if err := mgr.AddSender(key, name); err != nil {
			return err
		}
		fmt.Fprintf(out, "Added sender %q: %s\n", key, name)

	default:
		return fmt.Errorf("unknown entity type %q. Use minister, recipient, cc, or sender", entityType)
	}

	return nil
}

// --- LIST command ---

var configListCmd = &cobra.Command{
	Use:   "list [ministers|recipients|ccs|senders]",
	Short: "List config entries",
	Long: `List all ministers, recipients, CC recipients, or senders.

Examples:
  nac-service-media config list ministers
  nac-service-media config list recipients
  nac-service-media config list ccs
  nac-service-media config list senders`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigList,
}

func runConfigList(cmd *cobra.Command, args []string) error {
	cfg := GetConfig()
	if cfg == nil {
		return fmt.Errorf("config file not found. Run 'nac-service-media setup' first")
	}

	return RunConfigListWithDependencies(cfg, cfgFile, args[0], DefaultOutput)
}

// RunConfigListWithDependencies runs the list command with injected dependencies
func RunConfigListWithDependencies(cfg *config.Config, configPath, entityType string, out OutputWriter) error {
	mgr := config.NewConfigManager(cfg, configPath)
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	switch entityType {
	case "ministers":
		ministers := mgr.ListMinisters()
		if len(ministers) == 0 {
			fmt.Fprintln(out, "No ministers configured.")
			return nil
		}
		// Sort by key for consistent output
		sort.Slice(ministers, func(i, j int) bool {
			return ministers[i].Key < ministers[j].Key
		})
		fmt.Fprintln(w, "KEY\tNAME")
		for _, m := range ministers {
			fmt.Fprintf(w, "%s\t%s\n", m.Key, m.Name)
		}

	case "recipients":
		recipients := mgr.ListRecipients()
		if len(recipients) == 0 {
			fmt.Fprintln(out, "No recipients configured.")
			return nil
		}
		// Sort by key for consistent output
		sort.Slice(recipients, func(i, j int) bool {
			return recipients[i].Key < recipients[j].Key
		})
		fmt.Fprintln(w, "KEY\tNAME\tEMAIL")
		for _, r := range recipients {
			fmt.Fprintf(w, "%s\t%s\t%s\n", r.Key, r.Name, r.Address)
		}

	case "ccs":
		ccs := mgr.ListCCs()
		if len(ccs) == 0 {
			fmt.Fprintln(out, "No CCs configured.")
			return nil
		}
		fmt.Fprintln(w, "KEY\tNAME\tEMAIL")
		for _, c := range ccs {
			fmt.Fprintf(w, "%s\t%s\t%s\n", c.Key, c.Name, c.Address)
		}

	case "senders":
		senders := mgr.ListSenders()
		if len(senders) == 0 {
			fmt.Fprintln(out, "No senders configured.")
			return nil
		}
		// Sort by key for consistent output
		sort.Slice(senders, func(i, j int) bool {
			return senders[i].Key < senders[j].Key
		})
		// Show default indicator
		defaultSender := cfg.Senders.DefaultSender
		fmt.Fprintln(w, "KEY\tNAME\tDEFAULT")
		for _, s := range senders {
			isDefault := ""
			if s.Key == defaultSender {
				isDefault = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", s.Key, s.Name, isDefault)
		}

	default:
		return fmt.Errorf("unknown entity type %q. Use ministers, recipients, ccs, or senders", entityType)
	}

	return w.Flush()
}

// --- REMOVE command ---

var configRemoveCmd = &cobra.Command{
	Use:   "remove [minister|recipient|cc|sender] <key>",
	Short: "Remove a config entry",
	Long: `Remove a minister, recipient, CC, or sender from the configuration.

Examples:
  nac-service-media config remove minister smith
  nac-service-media config remove recipient jane
  nac-service-media config remove cc mary
  nac-service-media config remove sender avteam`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigRemove,
}

func runConfigRemove(cmd *cobra.Command, args []string) error {
	cfg := GetConfig()
	if cfg == nil {
		return fmt.Errorf("config file not found. Run 'nac-service-media setup' first")
	}

	return RunConfigRemoveWithDependencies(cfg, cfgFile, args[0], args[1], DefaultOutput)
}

// RunConfigRemoveWithDependencies runs the remove command with injected dependencies
func RunConfigRemoveWithDependencies(cfg *config.Config, configPath, entityType, key string, out OutputWriter) error {
	mgr := config.NewConfigManager(cfg, configPath)

	switch entityType {
	case "minister":
		if err := mgr.RemoveMinister(key); err != nil {
			return err
		}
		fmt.Fprintf(out, "Removed minister %q\n", key)

	case "recipient":
		if err := mgr.RemoveRecipient(key); err != nil {
			return err
		}
		fmt.Fprintf(out, "Removed recipient %q\n", key)

	case "cc":
		if err := mgr.RemoveCC(key); err != nil {
			return err
		}
		fmt.Fprintf(out, "Removed CC %q\n", key)

	case "sender":
		if err := mgr.RemoveSender(key); err != nil {
			return err
		}
		fmt.Fprintf(out, "Removed sender %q\n", key)

	default:
		return fmt.Errorf("unknown entity type %q. Use minister, recipient, cc, or sender", entityType)
	}

	return nil
}

// --- UPDATE command ---

var (
	updateName  string
	updateEmail string
)

var configUpdateCmd = &cobra.Command{
	Use:   "update [minister|recipient|cc|sender] <key>",
	Short: "Update a config entry",
	Long: `Update an existing minister, recipient, CC, or sender in the configuration.

Examples:
  nac-service-media config update minister smith --name "Rev. John H. Smith"
  nac-service-media config update recipient jane --email "jane.new@example.com"
  nac-service-media config update cc mary --name "Mary Smith" --email "mary.smith@example.com"
  nac-service-media config update sender avteam --name "White Plains A/V Team"`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigUpdate,
}

func init() {
	configUpdateCmd.Flags().StringVar(&updateName, "name", "", "New display name")
	configUpdateCmd.Flags().StringVar(&updateEmail, "email", "", "New email address")
}

func runConfigUpdate(cmd *cobra.Command, args []string) error {
	cfg := GetConfig()
	if cfg == nil {
		return fmt.Errorf("config file not found. Run 'nac-service-media setup' first")
	}

	if updateName == "" && updateEmail == "" {
		return fmt.Errorf("at least one of --name or --email is required")
	}

	return RunConfigUpdateWithDependencies(cfg, cfgFile, args[0], args[1], updateName, updateEmail, DefaultOutput)
}

// RunConfigUpdateWithDependencies runs the update command with injected dependencies
func RunConfigUpdateWithDependencies(cfg *config.Config, configPath, entityType, key, name, email string, out OutputWriter) error {
	mgr := config.NewConfigManager(cfg, configPath)

	switch entityType {
	case "minister":
		if name == "" {
			return fmt.Errorf("--name is required for minister update")
		}
		if err := mgr.UpdateMinister(key, name); err != nil {
			return err
		}
		fmt.Fprintf(out, "Updated minister %q\n", key)

	case "recipient":
		if err := mgr.UpdateRecipient(key, name, email); err != nil {
			return err
		}
		fmt.Fprintf(out, "Updated recipient %q\n", key)

	case "cc":
		if err := mgr.UpdateCC(key, name, email); err != nil {
			return err
		}
		fmt.Fprintf(out, "Updated CC %q\n", key)

	case "sender":
		if name == "" {
			return fmt.Errorf("--name is required for sender update")
		}
		if err := mgr.UpdateSender(key, name); err != nil {
			return err
		}
		fmt.Fprintf(out, "Updated sender %q\n", key)

	default:
		return fmt.Errorf("unknown entity type %q. Use minister, recipient, cc, or sender", entityType)
	}

	return nil
}
