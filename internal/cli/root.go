package cli

import "github.com/spf13/cobra"

func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "rtc-emulator",
		Short:         "CLI tool to emulate network impairments for WebRTC experiments",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.AddCommand(newLabCmd())

	return cmd
}
