package cmd

import (
	"github.com/davidboren/k8-spot-daemon/awscode"
	"github.com/davidboren/k8-spot-daemon/core"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Repeatedly pull spot instance pricing and adjust autoscaler if monitor flag is set to false",
	Long:  `Runs a loop monitoring the current instance pricing and, if the monitor flag is set to false, adjusts the autoscalingGroup accordingly.  If monitor is set to true, then it reports to standard out the autoscaler adjustments that it WOULD have made, had it been actually running`,
	Run: func(cmd *cobra.Command, args []string) {
		spotConfig := awscode.GetSpotConfigFromCommand(RootCmd)
		monitor, _ := cmd.PersistentFlags().GetBool("monitor")
		core.RunDaemon(monitor, spotConfig)
	}}
