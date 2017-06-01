package core

import (
	"fmt"
	"hash/fnv"
	"math"
	"strconv"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/davidboren/k8-spot-daemon/awscode"
	"github.com/davidboren/k8-spot-daemon/k8code"
	"github.com/davidboren/k8-spot-daemon/pricing"
)

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func UpdateLaunchConfiguration(sess *session.Session, spotConfig awscode.SpotConfig,
	priceList []pricing.FullSummary, podSummary map[string]float64,
	clientset *kubernetes.Clientset, monitor bool) bool {
	maxNodes := spotConfig.MaxAutoscalingNodes
	autoscalerName := spotConfig.AutoScalingGroupName
	autoscaler := awscode.GetAutoscaler(sess, autoscalerName)
	allLaunchConfigurations := awscode.GetLaunchConfigurations(sess, spotConfig.LaunchConfigurationPrefix)
	var launchConfiguration *autoscaling.LaunchConfiguration
	for _, lc := range allLaunchConfigurations {
		if *lc.LaunchConfigurationName == *autoscaler.LaunchConfigurationName {
			launchConfiguration = lc
		}
	}

	if launchConfiguration == nil {
		panic("Could not identify launchConfiguration currently in use by autoscaler...")
	}

	originalSpotPrice, _ := strconv.ParseFloat(*launchConfiguration.SpotPrice, 64)
	newInstanceType := *launchConfiguration.InstanceType
	newSpotPrice := originalSpotPrice
	maxMemoryRequired := (1 + spotConfig.MemoryBufferPercentage*0.01) * podSummary["maxMemoryRequestedGB"]

	scaleMemory := false
	originalDollarsPerHour := spotConfig.MaxTotalDollarsPerHour
	for _, instanceSummary := range priceList {
		if instanceSummary.Name == *launchConfiguration.InstanceType {
			nodesNeeded := math.Max(1, math.Ceil(podSummary["totalMemoryRequestedGB"]/instanceSummary.Mem))
			currentSpotPrice := math.Ceil(100.0*math.Max(
				instanceSummary.Price*(1.0+spotConfig.MinMarkupPercentage*0.01),
				instanceSummary.Price+2.97*instanceSummary.StdDev)) / 100.0
			originalDollarsPerHour = math.Min(float64(nodesNeeded), float64(maxNodes)) * currentSpotPrice
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
		currentSpotPrice := math.Ceil(100.0*math.Max(
			instanceSummary.Price*(1.0+spotConfig.MinMarkupPercentage*0.01),
			instanceSummary.Price+2.97*instanceSummary.StdDev)) / 100.0
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
		if scaleMemory ||
			math.Abs(minActualDollarsPerHour-originalDollarsPerHour) > (0.01*spotConfig.MinPriceDifferencePercentage)*originalDollarsPerHour {
			newSpotPriceString := strconv.FormatFloat(newSpotPrice, 'f', 2, 64)
			fmt.Printf("\nOriginal Configuration:\n        InstanceType: '%v'\n        SpotPrice: '%v'\n",
				*launchConfiguration.InstanceType,
				*launchConfiguration.SpotPrice)
			fmt.Printf("New Configuration:\n        InstanceType: '%v'\n        SpotPrice: '%v'\n",
				newInstanceType,
				newSpotPriceString)
			fmt.Printf("Total $ per Hour: '%v'\n",
				minActualDollarsPerHour)
			createLaunchConfigurationInput := awscode.DuplicateLaunchConfiguration(launchConfiguration)
			createLaunchConfigurationInput.SetSpotPrice(newSpotPriceString)
			createLaunchConfigurationInput.SetInstanceType(newInstanceType)
			t := time.Now()
			newName := spotConfig.LaunchConfigurationPrefix + "-" + fmt.Sprintf("%v", hash(t.String()))
			createLaunchConfigurationInput.SetLaunchConfigurationName(newName)

			updateAutoScalingGroupInput := autoscaling.UpdateAutoScalingGroupInput{
				AutoScalingGroupName:    autoscaler.AutoScalingGroupName,
				LaunchConfigurationName: launchConfiguration.LaunchConfigurationName}
			if !monitor {
				fmt.Printf("Launchconfiguration '%v' will be created\n", *createLaunchConfigurationInput.LaunchConfigurationName)
				fmt.Printf("Creating LaunchConfiguration with input:\n%v\n", createLaunchConfigurationInput)

				// autoscaling_svc := autoscaling.New(sess)
				// _, create_lc_err := autoscaling_svc.CreateLaunchConfiguration(&createLaunchConfigurationInput)
				// if create_lc_err != nil {
				// 	panic(create_lc_err)
				// }

				fmt.Printf("\nAutoScalingGroup '%v' will be updated\n", *autoscaler.AutoScalingGroupName)
				fmt.Printf("Updating AutoScalingGroup with input:\n%v\n", updateAutoScalingGroupInput)
				// _, update_asg_err := autoscaling_svc.UpdateAutoScalingGroup(&updateAutoScalingGroupInput)
				// if update_asg_err != nil {
				// 	panic(update_asg_err)
				// }

				for _, lc := range allLaunchConfigurations {
					if newName != *lc.LaunchConfigurationName {
						deleteLaunchConfigurationInput := autoscaling.DeleteLaunchConfigurationInput{
							LaunchConfigurationName: lc.LaunchConfigurationName}
						fmt.Printf("\nLaunchconfiguration '%v' will be deleted\n", *lc.LaunchConfigurationName)
						fmt.Printf("\nDeleting LaunchConfiguration with input:\n%v\n", deleteLaunchConfigurationInput)
						// _, delete_lc_err := autoscaling_svc.DeleteLaunchConfiguration(&deleteLaunchConfigurationInput)
						// if delete_lc_err != nil {
						// 	panic(delete_lc_err)
						// }
					}
				}
			} else {
				fmt.Printf("")
				fmt.Printf("Launchconfiguration '%v' would be created\n", *createLaunchConfigurationInput.LaunchConfigurationName)
				fmt.Printf("AutoscalingGroup '%v' would be updated\n", *autoscaler.AutoScalingGroupName)
				for _, lc := range allLaunchConfigurations {
					if newName != *lc.LaunchConfigurationName {
						fmt.Printf("Launchconfiguration '%v' would be deleted\n", *lc.LaunchConfigurationName)
					}
				}
			}
			return true
		}
		return false
	} else {
		return false

	}
}

func RunDaemon(monitor bool, spotConfig awscode.SpotConfig) {
	for {
		updated := false
		fmt.Printf("Checking prices at %v\n", time.Now())
		clientset := k8code.GetClientSet()
		sess := session.Must(session.NewSession(&aws.Config{
			Region: aws.String(spotConfig.RegionName),
		}))

		podSummary := k8code.SummarizePods(clientset)
		fmt.Printf("Kubernetes Usage:\n")
		fmt.Printf(
			"    Total Memory Requested: %12.3f GB || Total Memory in Use: %12.3f GB || Max Memory: %8.3f GB || Num Pods: %v\n",
			podSummary["totalMemoryRequestedGB"],
			podSummary["totalMemoryUsedGB"],
			podSummary["maxMemoryUsedGB"],
			int(podSummary["totalRunningPods"]))
		fmt.Printf("")

		if int(podSummary["totalRunningPods"]) < spotConfig.MaxPodKills {
			priceList := pricing.DescribePricing(sess, spotConfig)
			updated = UpdateLaunchConfiguration(sess, spotConfig, priceList, podSummary, clientset, monitor)
		} else {
			fmt.Printf("Too many active pods (%v) to turn over cluster...\n", int(podSummary["totalRunningPods"]))
		}

		if updated {
			fmt.Printf("AutoScalingGroup was updated.  Sleeping for '%v' seconds.\n", int(spotConfig.MinimumTurnoverSeconds))
			time.Sleep(time.Second * time.Duration(spotConfig.MinimumTurnoverSeconds))
		} else {
			fmt.Printf("AutoScalingGroup was not updated.  Sleeping for '%v' seconds.\n", int(spotConfig.UpdateIntervalSeconds))
			time.Sleep(time.Second * time.Duration(spotConfig.UpdateIntervalSeconds))
		}
	}
}
