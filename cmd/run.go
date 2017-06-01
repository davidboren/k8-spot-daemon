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
	Short: "Repeatedly pull spot instance pricing and adjust autoscaler if monitor flag is not set",
	Long:  `Runs a loop monitoring the current instance pricing and, if the monitor flag is not set, adjusts the autoScalingGroup accordingly.  If monitor is set to true, then it reports to standard out the autoscaler adjustments that it WOULD have made, had it been actually running`,
	Run: func(cmd *cobra.Command, args []string) {
		spotConfig := awscode.GetSpotConfigFromCommand(RootCmd)

		if len(spotConfig.AutoScalingGroupName) == 0 {
			panic("You must set an AutoScalingGroupName (--autoScalingGroupName or -q) to use this daemon")
		}

		if len(spotConfig.AutoScalingGroupName) == 0 {
			panic("You must set an LaunchConfigurationPrefix (--launchConfigurationPrefix or -l) to use this daemon")
		}

		monitor, _ := RootCmd.PersistentFlags().GetBool("monitor")
		core.RunDaemon(monitor, spotConfig)
	}}
