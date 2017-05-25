package main

import (
	"fmt"
	"math"
	"strconv"

	"k8s.io/client-go/kubernetes"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/davidboren/k8-spot-daemon/awscode"
	instanceConfig "github.com/davidboren/k8-spot-daemon/config"
	"github.com/davidboren/k8-spot-daemon/k8code"
	"github.com/davidboren/k8-spot-daemon/pricing"
	yaml "gopkg.in/yaml.v2"
)

var (
	VERSION string
	BUILD   string
)

func getSpotConfig() awscode.SpotConfig {
	asset, _ := instanceConfig.Asset("config/spotConfig.yaml")
	var spotConfig awscode.SpotConfig

	yaml.Unmarshal(asset, &spotConfig)
	return spotConfig

}

func UpdateLaunchConfiguration(sess *session.Session, spotConfig awscode.SpotConfig,
	priceList []pricing.FullSummary, podSummary map[string]float64, clientset *kubernetes.Clientset) {
	maxNodes := spotConfig.MaxAutoscalingNodes
	autoscalerName := spotConfig.AutoscalingGroupName
	autoscaler := awscode.GetAutoscaler(sess, autoscalerName)
	launchConfiguration := awscode.GetLaunchConfiguration(sess, *autoscaler.LaunchConfigurationName)

	originalSpotPrice, _ := strconv.ParseFloat(*launchConfiguration.SpotPrice, 64)
	newInstanceType := *launchConfiguration.InstanceType
	newSpotPrice := originalSpotPrice
	maxMemoryRequired := (1 + spotConfig.MemoryBufferPercentage*0.01) * podSummary["maxMemoryRequestedGB"]

	scaleMemory := false
	for _, instanceSummary := range priceList {
		if instanceSummary.Name == *launchConfiguration.InstanceType {
			if instanceSummary.Mem < maxMemoryRequired {
				scaleMemory = true
			}
			break
		}
	}

	minActualDollarsPerHour := spotConfig.MaxTotalDollarsPerHour
	modified := false
	for _, instanceSummary := range priceList {
		maxTotalDollarsPerHour := float64(maxNodes) * instanceSummary.Price
		nodesNeeded := math.Max(1, math.Ceil(podSummary["totalMemoryRequestedGB"]/instanceSummary.Mem))
		currentSpotPrice := math.Max(
			instanceSummary.Price*(1.0+spotConfig.MinMarkupPercentage*0.01),
			instanceSummary.Price+2.97*instanceSummary.StdDev)
		actualDollarsPerHour := math.Min(float64(nodesNeeded), float64(maxNodes)) * currentSpotPrice
		if instanceSummary.Mem >= maxMemoryRequired {
			if maxTotalDollarsPerHour < spotConfig.MaxTotalDollarsPerHour {
				if instanceSummary.PricePerGB < spotConfig.MaxDollarsPerGB {
					if instanceSummary.PricePerCPU < spotConfig.MaxDollarsPerCPU {
						if instanceSummary.CoefVar < spotConfig.MaxCV {
							if actualDollarsPerHour < minActualDollarsPerHour {
								minActualDollarsPerHour = actualDollarsPerHour
								newInstanceType = instanceSummary.Name
								newSpotPrice = currentSpotPrice
								modified = true
							}
						}
					}
				}
			}
		}
	}

	if modified && int(podSummary["totalRunningPods"]) <= spotConfig.MaxPodKills {
		if scaleMemory || newInstanceType != *launchConfiguration.InstanceType ||
			math.Abs(newSpotPrice-originalSpotPrice) > (0.01*spotConfig.MinPriceDifferencePercentage)*originalSpotPrice {
			newSpotPriceString := strconv.FormatFloat(newSpotPrice, 'f', 2, 64)
			createLaunchConfigurationInput := awscode.DuplicateLaunchConfiguration(launchConfiguration)
			createLaunchConfigurationInput.SetSpotPrice(newSpotPriceString)
			createLaunchConfigurationInput.SetInstanceType(newInstanceType)
			// fmt.Printf("Creating LaunchConfiguration with input:\n%v\n", createLaunchConfigurationInput)

			// fmt.Printf("Deleting LaunchConfiguration with input:\n%v\n", deleteLaunchConfigurationInput)
			// deleteLaunchConfigurationInput := autoscaling.DeleteLaunchConfigurationInput{
			// 	LaunchConfigurationName: launchConfiguration.LaunchConfigurationName}

			fmt.Printf("Launchconfiguration '%v' modified\n\n", *launchConfiguration.LaunchConfigurationName)
			fmt.Printf("Original Configuration:\nInstanceType: '%v'\nSpotPrice: '%v'\n\n",
				*launchConfiguration.InstanceType,
				*launchConfiguration.SpotPrice)
			fmt.Printf("New Configuration:\nInstanceType: '%v'\nSpotPrice: '%v'\n",
				newInstanceType,
				newSpotPriceString)
			fmt.Printf("\n$ per Hour: '%v'\n",
				minActualDollarsPerHour)
		}
	}
}

func main() {

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	clientset := k8code.GetClientSet()
	spotConfig := getSpotConfig()

	podSummary := k8code.SummarizePods(clientset)
	fmt.Printf(
		"Total Memory Requested: %12.3f GB || Total Memory in Use: %12.3f GB || Max Memory: %8.3f GB || Num Pods: %v\n",
		podSummary["totalMemoryRequestedGB"],
		podSummary["totalMemoryUsedGB"],
		podSummary["maxMemoryUsedGB"],
		int(podSummary["totalRunningPods"]))

	if int(podSummary["totalRunningPods"]) < spotConfig.MaxPodKills {
		priceList := pricing.DescribePricing(sess, spotConfig)
		UpdateLaunchConfiguration(sess, spotConfig, priceList, podSummary, clientset)
	} else {
		fmt.Printf("Too many active pods (%v) to turn over cluster...\n", int(podSummary["totalRunningPods"]))
	}

}
