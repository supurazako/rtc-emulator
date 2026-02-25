package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/supurazako/rtc-emulator/internal/lab"
)

func newLabCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lab",
		Short: "Manage local lab environments",
	}

	cmd.AddCommand(
		newLabCreateCmd(),
		newLabApplyCmd(),
		newLabShowCmd(),
		newLabDestroyCmd(),
	)

	return cmd
}

func newLabCreateCmd() *cobra.Command {
	var nodes int

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a lab environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := lab.Create(context.Background(), lab.CreateOptions{Nodes: nodes})
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "created bridge=%s nodes=%d\n", result.Bridge, len(result.Nodes))
			for _, node := range result.Nodes {
				fmt.Fprintf(cmd.OutOrStdout(), "- %s ip=%s\n", node.Name, node.IP)
			}
			if result.InternetReachable {
				fmt.Fprintln(cmd.OutOrStdout(), "internet-check=ok")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "internet-check=skipped-or-unreachable (host bridge connectivity is confirmed)")
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&nodes, "nodes", 1, "number of nodes to create")

	return cmd
}

func newLabApplyCmd() *cobra.Command {
	var node string
	var delay string
	var loss string
	var jitter string
	var bw string

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply impairments to a node",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}

	cmd.Flags().StringVar(&node, "node", "", "target node")
	cmd.Flags().StringVar(&delay, "delay", "", "delay setting")
	cmd.Flags().StringVar(&loss, "loss", "", "packet loss setting")
	cmd.Flags().StringVar(&jitter, "jitter", "", "jitter setting")
	cmd.Flags().StringVar(&bw, "bw", "", "bandwidth setting")
	_ = cmd.MarkFlagRequired("node")

	return cmd
}

func newLabShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current lab state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}
}

func newLabDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Short: "Destroy lab environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := lab.Destroy(context.Background())
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "destroyed bridge=%t nodes=%d\n", result.BridgeDeleted, len(result.NodesDeleted))
			for _, node := range result.NodesDeleted {
				fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", node)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "state-missing-fallback=%t\n", result.StateMissingFallback)
			fmt.Fprintf(cmd.OutOrStdout(), "ip-forward-restored=%t\n", result.IPForwardRestored)
			return nil
		},
	}
}
