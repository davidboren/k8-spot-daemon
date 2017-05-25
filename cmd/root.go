package cmd

import (
	"fmt"
	"os"

	"github.com/davidboren/k8-spot-daemon/awscode"
	"github.com/davidboren/k8-spot-daemon/core"
	"github.com/spf13/cobra"
)

var spotConfig awscode.SpotConfig
var monitor bool

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "k8-spot-daemon",
	Short: "Monitor spot instance pricing and adjust your kubernetes cluster autoscaler accordingly.",
	Long: `K8-Spot-Daemon is a CLI library for Go that allows for simple adjustment of your aws autoscaler
in accordance with the needs of your kubernetes cluster.`,
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {

	spotConfig = core.GetSpotConfig()

	RootCmd.PersistentFlags().StringVarP(
		&spotConfig.AutoscalingGroupName,
		"autoscalingGroupName",
		"q",
		spotConfig.AutoscalingGroupName,
		"Set your aws Autoscaling Group Name")

	RootCmd.PersistentFlags().IntVarP(
		&spotConfig.MaxAutoscalingNodes,
		"maxAutoscalingNodes",
		"n",
		spotConfig.MaxAutoscalingNodes,
		"Set the maximum number of autoscaling nodes here. (Used for totalDollars/Hour calculation only)")

	spotConfig.HistoricalHours = *RootCmd.PersistentFlags().Float64P(
		"historicalHours",
		"s",
		spotConfig.HistoricalHours,
		"Set the hours over which spot instance price data should be averaged (in a weighted fashion)")

	RootCmd.PersistentFlags().StringVarP(
		&spotConfig.RegionName,
		"regionName",
		"e",
		spotConfig.RegionName,
		"Set the Region in which to monitor spot pricing data")

	spotConfig.MaxCV = *RootCmd.PersistentFlags().Float64P(
		"maxCV",
		"c",
		spotConfig.MaxCV,
		"Set the Maximum coefficient of variation (of spotprice within the hour-window) allowable for an instance type to be considered for a switch.")

	spotConfig.MinGB = *RootCmd.PersistentFlags().Float64P(
		"minGB",
		"y",
		spotConfig.MinGB,
		"Set the Minimum GB memory necessary for an instance type to be considered for a switch.")

	spotConfig.MaxDollarsPerGB = *RootCmd.PersistentFlags().Float64P(
		"maxDollarsPerGB",
		"g",
		spotConfig.MaxDollarsPerGB,
		"Set the Maximum hourly Dollars per GB of memory allowable for an instance type to be considered for a switch.")

	spotConfig.MaxDollarsPerCPU = *RootCmd.PersistentFlags().Float64P(
		"maxDollarsPerCPU",
		"p",
		spotConfig.MaxDollarsPerCPU,
		"Set the Maximum hourly Dollars per CPU allowable for an instance type to be considered for a switch.")

	RootCmd.PersistentFlags().IntVarP(
		&spotConfig.MaxPodKills,
		"maxPodKills",
		"k",
		spotConfig.MaxPodKills,
		"Set the Maximum number of running pods that we are allowed to kill with an autoscaler instance-type switch.")

	spotConfig.MaxTotalDollarsPerHour = *RootCmd.PersistentFlags().Float64P(
		"maxTotalDollarsPerHour",
		"t",
		spotConfig.MaxTotalDollarsPerHour,
		"Set the Maximum dollars per hour that we can spend on the autoscaler (takes into account maxAutoscalingNodes)")

	spotConfig.MinMarkupPercentage = *RootCmd.PersistentFlags().Float64P(
		"minMarkupPercentage",
		"r",
		spotConfig.MinMarkupPercentage,
		"Set the Minimum markup percentage (over the current averaged spotprice) allowed when choosing a bid price")

	spotConfig.MemoryBufferPercentage = *RootCmd.PersistentFlags().Float64P(
		"memoryBufferPercentage",
		"b",
		spotConfig.MemoryBufferPercentage,
		"Set the percentage of memory reserved for the kubernetes system on each machine.")

	spotConfig.MinPriceDifferencePercentage = *RootCmd.PersistentFlags().Float64P(
		"minPriceDifferencePercentage",
		"d",
		spotConfig.MinPriceDifferencePercentage,
		"Set the minimum bid price difference necessary to make an alteration to the bid price.")

	spotConfig.UpdateIntervalSeconds = *RootCmd.PersistentFlags().Float64P(
		"updateIntervalSeconds",
		"u",
		spotConfig.UpdateIntervalSeconds,
		"Set the seconds to sleep between each spot price check.")

	RootCmd.PersistentFlags().BoolVarP(
		&monitor,
		"monitor",
		"o",
		false,
		"Whether or not the autoscaler should be updated (false), or the adjustments merely monitored and reported to standard out.")

}
