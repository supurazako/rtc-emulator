package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

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
		newLabImpairCmd(),
		newLabScenarioCmd(),
		newLabWebRTCCmd(),
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

func newLabImpairCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "impair",
		Short: "Manage node network impairments",
	}

	cmd.AddCommand(
		newLabImpairApplyCmd(),
		newLabImpairClearCmd(),
	)

	return cmd
}

func newLabImpairApplyCmd() *cobra.Command {
	var node string
	var delay string
	var loss string
	var jitter string
	var bw string

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply impairments to a node",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := lab.Apply(context.Background(), lab.ApplyOptions{
				Node:   node,
				Delay:  delay,
				Loss:   loss,
				Jitter: jitter,
				BW:     bw,
			})
			if err != nil {
				return err
			}

			printApplyResult(cmd, result)
			return nil
		},
	}

	addImpairApplyFlags(cmd, &node, &delay, &loss, &jitter, &bw)

	return cmd
}

func newLabImpairClearCmd() *cobra.Command {
	var node string

	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear impairments from a node",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := lab.Clear(context.Background(), lab.ClearOptions{Node: node})
			if err != nil {
				return err
			}

			status := "absent"
			if result.Cleared {
				status = "cleared"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "cleared node=%s qdisc=%s\n", result.Node, status)
			return nil
		},
	}

	cmd.Flags().StringVar(&node, "node", "", "target node")
	_ = cmd.MarkFlagRequired("node")

	return cmd
}

func newLabScenarioCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scenario",
		Short: "Run WebRTC-oriented lab scenarios",
	}

	cmd.AddCommand(newLabScenarioRunCmd())

	return cmd
}

func newLabScenarioRunCmd() *cobra.Command {
	var runsDir string
	var node string
	var delay string
	var loss string
	var jitter string
	var bw string

	cmd := &cobra.Command{
		Use:   "run SCENARIO",
		Short: "Run a named lab scenario and save event logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := lab.RunScenario(context.Background(), lab.ScenarioRunOptions{
				Scenario: args[0],
				RunsDir:  runsDir,
				Node:     node,
				Delay:    delay,
				Loss:     loss,
				Jitter:   jitter,
				BW:       bw,
			})
			if result != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "run-id=%s\n", result.RunID)
				fmt.Fprintf(cmd.OutOrStdout(), "run-dir=%s\n", result.RunDir)
				fmt.Fprintf(cmd.OutOrStdout(), "events=%s\n", result.EventsPath)
			}
			return err
		},
	}

	cmd.Flags().StringVar(&runsDir, "runs-dir", "runs", "directory for scenario run outputs")
	cmd.Flags().StringVar(&node, "node", "node1", "target node")
	cmd.Flags().StringVar(&delay, "delay", "", "delay setting")
	cmd.Flags().StringVar(&loss, "loss", "", "packet loss setting")
	cmd.Flags().StringVar(&jitter, "jitter", "", "jitter setting")
	cmd.Flags().StringVar(&bw, "bw", "", "bandwidth setting")

	return cmd
}

func newLabWebRTCCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webrtc",
		Short: "Run lab WebRTC workflows",
	}

	cmd.AddCommand(
		newLabWebRTCP2PCmd(),
		newLabWebRTCPeerCmd(),
	)

	return cmd
}

func newLabWebRTCP2PCmd() *cobra.Command {
	var runsDir string
	var nodeA string
	var nodeB string
	var duration time.Duration
	var statsInterval time.Duration

	cmd := &cobra.Command{
		Use:   "p2p",
		Short: "Run a lab WebRTC P2P flow and save stats logs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := lab.RunWebRTCP2P(context.Background(), lab.WebRTCP2POptions{
				RunsDir:       runsDir,
				NodeA:         nodeA,
				NodeB:         nodeB,
				Duration:      duration,
				StatsInterval: statsInterval,
			})
			if result != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "run-id=%s\n", result.RunID)
				fmt.Fprintf(cmd.OutOrStdout(), "run-dir=%s\n", result.RunDir)
				fmt.Fprintf(cmd.OutOrStdout(), "events=%s\n", result.EventsPath)
				fmt.Fprintf(cmd.OutOrStdout(), "stats=%s\n", result.StatsPath)
			}
			return err
		},
	}

	cmd.Flags().StringVar(&runsDir, "runs-dir", "runs", "directory for WebRTC run outputs")
	cmd.Flags().StringVar(&nodeA, "node-a", "node1", "offerer node")
	cmd.Flags().StringVar(&nodeB, "node-b", "node2", "answerer node")
	cmd.Flags().DurationVar(&duration, "duration", 10*time.Second, "stats collection duration")
	cmd.Flags().DurationVar(&statsInterval, "stats-interval", time.Second, "stats collection interval")

	return cmd
}

func newLabWebRTCPeerCmd() *cobra.Command {
	var role string
	var runID string
	var runDir string
	var node string
	var peer string
	var duration time.Duration
	var statsInterval time.Duration

	cmd := &cobra.Command{
		Use:    "peer",
		Short:  "Run one internal WebRTC peer process",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return lab.RunWebRTCPeer(context.Background(), lab.WebRTCPeerOptions{
				Role:          role,
				RunID:         runID,
				RunDir:        runDir,
				Node:          node,
				Peer:          peer,
				Duration:      duration,
				StatsInterval: statsInterval,
			})
		},
	}

	cmd.Flags().StringVar(&role, "role", "", "peer role")
	cmd.Flags().StringVar(&runID, "run-id", "", "run id")
	cmd.Flags().StringVar(&runDir, "run-dir", "", "run directory")
	cmd.Flags().StringVar(&node, "node", "", "current node")
	cmd.Flags().StringVar(&peer, "peer", "", "remote peer node")
	cmd.Flags().DurationVar(&duration, "duration", 10*time.Second, "stats collection duration")
	cmd.Flags().DurationVar(&statsInterval, "stats-interval", time.Second, "stats collection interval")
	_ = cmd.MarkFlagRequired("role")
	_ = cmd.MarkFlagRequired("run-id")
	_ = cmd.MarkFlagRequired("run-dir")
	_ = cmd.MarkFlagRequired("node")
	_ = cmd.MarkFlagRequired("peer")

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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := lab.Apply(context.Background(), lab.ApplyOptions{
				Node:   node,
				Delay:  delay,
				Loss:   loss,
				Jitter: jitter,
				BW:     bw,
			})
			if err != nil {
				return err
			}

			printApplyResult(cmd, result)
			return nil
		},
	}

	addImpairApplyFlags(cmd, &node, &delay, &loss, &jitter, &bw)

	return cmd
}

func addImpairApplyFlags(cmd *cobra.Command, node *string, delay *string, loss *string, jitter *string, bw *string) {
	cmd.Flags().StringVar(node, "node", "", "target node")
	cmd.Flags().StringVar(delay, "delay", "", "delay setting")
	cmd.Flags().StringVar(loss, "loss", "", "packet loss setting")
	cmd.Flags().StringVar(jitter, "jitter", "", "jitter setting")
	cmd.Flags().StringVar(bw, "bw", "", "bandwidth setting")
	_ = cmd.MarkFlagRequired("node")
}

func printApplyResult(cmd *cobra.Command, result *lab.ApplyResult) {
	display := func(v string) string {
		if v == "" {
			return "-"
		}
		return v
	}
	fmt.Fprintf(
		cmd.OutOrStdout(),
		"applied node=%s delay=%s loss=%s jitter=%s bw=%s\n",
		result.Node,
		display(result.Delay),
		display(result.Loss),
		display(result.Jitter),
		display(result.BW),
	)
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
