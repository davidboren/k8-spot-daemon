package core

import (
	"crypto/md5"
	"fmt"
	"io"
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

func hash(s string) string {
	h := md5.New()
	io.WriteString(h, s)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func getAdjustedSpotPrice(instanceSummary pricing.FullSummary, spotConfig awscode.SpotConfig) float64 {
	return math.Ceil(100.0*math.Max(
		instanceSummary.Price*(1.0+spotConfig.MinMarkupPercentage*0.01),
		instanceSummary.Price+2.97*instanceSummary.StdDev)) / 100.0

}

func getNodesNeeded(instanceSummary pricing.FullSummary, podSummary map[string]float64) int {
	return int(math.Max(1, math.Ceil(podSummary["totalMemoryRequestedGB"]/instanceSummary.Mem)))
}
func getDollarsPerHour(instanceSummary pricing.FullSummary, nodesNeeded int, maxNodes int, currentSpotPrice float64) float64 {
	return math.Min(float64(nodesNeeded), float64(maxNodes)) * currentSpotPrice
}

func checkOriginalMemoryAndPrice(priceList []pricing.FullSummary,
	spotConfig awscode.SpotConfig, podSummary map[string]float64, originalInstanceType string,
	originalSpotPrice float64, maxMemoryRequired float64) (bool, float64) {

	originalDollarsPerHour := 0.0
	foundOriginal := false
	scaleMemory := false

	for _, instanceSummary := range priceList {
		if instanceSummary.Name == originalInstanceType {
			foundOriginal = true
			nodesNeeded := getNodesNeeded(instanceSummary, podSummary)
			originalDollarsPerHour = getDollarsPerHour(
				instanceSummary, nodesNeeded, spotConfig.MaxAutoscalingNodes, originalSpotPrice)
			if instanceSummary.Mem < maxMemoryRequired {
				scaleMemory = true
			}
			break
		}
	}
	if !foundOriginal {
		panic("Could not identify original instanceType in our pricing list...")
	}
	return scaleMemory, originalDollarsPerHour

}

func getBestFilteredType(originalInstanceType string, originalSpotPrice float64, spotConfig awscode.SpotConfig,
	priceList []pricing.FullSummary, maxMemoryRequired float64, maxNodes int,
	podSummary map[string]float64) (string, float64, float64, bool) {

	newInstanceType := originalInstanceType
	newSpotPrice := originalSpotPrice
	minActualDollarsPerHour := spotConfig.MaxTotalDollarsPerHour
	anySatisfyConstraints := false
	for _, instanceSummary := range priceList {
		maxTotalDollarsPerHour := float64(maxNodes) * instanceSummary.Price
		nodesNeeded := getNodesNeeded(instanceSummary, podSummary)
		currentSpotPrice := getAdjustedSpotPrice(instanceSummary, spotConfig)
		actualDollarsPerHour := getDollarsPerHour(
			instanceSummary, nodesNeeded, spotConfig.MaxAutoscalingNodes, currentSpotPrice)
		if instanceSummary.Mem >= spotConfig.MinGB {
			if instanceSummary.Mem >= maxMemoryRequired {
				if maxTotalDollarsPerHour < spotConfig.MaxTotalDollarsPerHour {
					if instanceSummary.PricePerGB < spotConfig.MaxDollarsPerGB {
						if instanceSummary.PricePerCPU < spotConfig.MaxDollarsPerCPU {
							if instanceSummary.CoefVar < spotConfig.MaxCV {
								if actualDollarsPerHour < minActualDollarsPerHour {
									minActualDollarsPerHour = actualDollarsPerHour
									newInstanceType = instanceSummary.Name
									newSpotPrice = currentSpotPrice
									anySatisfyConstraints = true
								}
							}
						}
					}
				}
			}
		}
	}
	return newInstanceType, newSpotPrice, minActualDollarsPerHour, anySatisfyConstraints
}

func GetNewLaunchConfigurationName(prefix string) string {
	return prefix + "-" + fmt.Sprintf("%v", hash(time.Now().String()))
}

func UpdateLaunchConfiguration(autoscalingGroup *autoscaling.Group, launchConfiguration *autoscaling.LaunchConfiguration,
	allLaunchConfigurations []*autoscaling.LaunchConfiguration, spotConfig awscode.SpotConfig,
	minActualDollarsPerHour float64, newSpotPrice float64, newInstanceType string, monitor bool) {

	newSpotPriceString := strconv.FormatFloat(newSpotPrice, 'f', 2, 64)
	fmt.Printf("\nOriginal Configuration:\n        InstanceType: '%v'\n        SpotPrice: '%v'\n",
		*launchConfiguration.InstanceType,
		*launchConfiguration.SpotPrice)
	fmt.Printf("New Configuration:\n        InstanceType: '%v'\n        SpotPrice: '%v'\n",
		newInstanceType,
		newSpotPriceString)
	fmt.Printf("Total $ per Hour: '%v'\n",
		minActualDollarsPerHour)

	newLaunchConfigurationName := GetNewLaunchConfigurationName(spotConfig.LaunchConfigurationPrefix)

	createLaunchConfigurationInput := awscode.DuplicateLaunchConfiguration(launchConfiguration)
	createLaunchConfigurationInput.SetSpotPrice(newSpotPriceString)
	createLaunchConfigurationInput.SetInstanceType(newInstanceType)
	createLaunchConfigurationInput.SetLaunchConfigurationName(newLaunchConfigurationName)

	updateAutoScalingGroupInput := autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName:    autoscalingGroup.AutoScalingGroupName,
		LaunchConfigurationName: &newLaunchConfigurationName}
	creation_term := "would"
	if !monitor {
		creation_term = "will"
	} else {
		fmt.Printf("Monitoring only...\n")
	}

	fmt.Printf("Launchconfiguration '%v' %v be created with input: \n%v\n",
		*createLaunchConfigurationInput.LaunchConfigurationName, creation_term, createLaunchConfigurationInput)

	if !monitor {
		// autoscaling_svc := autoscaling.New(sess)
		// _, create_lc_err := autoscaling_svc.CreateLaunchConfiguration(&createLaunchConfigurationInput)
		// if create_lc_err != nil {
		// 	panic(create_lc_err)
		// }
	}

	fmt.Printf("AutoScalingGroup '%v' %v be updated with input: \n%v\n",
		*autoscalingGroup.AutoScalingGroupName, creation_term, updateAutoScalingGroupInput)
	if !monitor {
		// _, update_asg_err := autoscaling_svc.UpdateAutoScalingGroup(&updateAutoScalingGroupInput)
		// if update_asg_err != nil {
		// 	panic(update_asg_err)
		// }
	}

	for _, lc := range allLaunchConfigurations {
		if newLaunchConfigurationName != *lc.LaunchConfigurationName {
			deleteLaunchConfigurationInput := autoscaling.DeleteLaunchConfigurationInput{
				LaunchConfigurationName: lc.LaunchConfigurationName}
			fmt.Printf("Launchconfiguration '%v' %v be deleted with input: \n%v\n",
				*lc.LaunchConfigurationName, creation_term, deleteLaunchConfigurationInput)
			if !monitor {
				// _, delete_lc_err := autoscaling_svc.DeleteLaunchConfiguration(&deleteLaunchConfigurationInput)
				// if delete_lc_err != nil {
				// 	panic(delete_lc_err)
				// }
			}
		}
	}
}

func CheckAndUpdate(sess *session.Session, spotConfig awscode.SpotConfig,
	priceList []pricing.FullSummary, podSummary map[string]float64,
	clientset *kubernetes.Clientset, monitor bool) bool {
	autoScalingGroupName := spotConfig.AutoScalingGroupName
	autoScalingGroup := awscode.GetAutoscaler(sess, autoScalingGroupName)
	allLaunchConfigurations := awscode.GetLaunchConfigurations(sess, spotConfig.LaunchConfigurationPrefix)
	var launchConfiguration *autoscaling.LaunchConfiguration
	for _, lc := range allLaunchConfigurations {
		if *lc.LaunchConfigurationName == *autoScalingGroup.LaunchConfigurationName {
			launchConfiguration = lc
		}
	}

	if launchConfiguration == nil {
		panic("Could not identify launchConfiguration currently in use by autoscaler...")
	}

	maxMemoryRequired := (1 + spotConfig.MemoryBufferPercentage*0.01) * podSummary["maxMemoryRequestedGB"]

	originalInstanceType := *launchConfiguration.InstanceType
	originalSpotPrice, price_err := strconv.ParseFloat(*launchConfiguration.SpotPrice, 64)
	if price_err != nil {
		panic(price_err)
	}
	scaleMemory, originalDollarsPerHour := checkOriginalMemoryAndPrice(priceList, spotConfig,
		podSummary, originalInstanceType, originalSpotPrice, maxMemoryRequired)

	newInstanceType, newSpotPrice, minActualDollarsPerHour, anySatisfyConstraints := getBestFilteredType(
		originalInstanceType, originalSpotPrice, spotConfig, priceList, maxMemoryRequired,
		spotConfig.MaxAutoscalingNodes, podSummary)

	minDollarsPerHourDifference := (0.01 * spotConfig.MinPriceDifferencePercentage) * originalDollarsPerHour
	passesDollarDifference := math.Abs(minActualDollarsPerHour-originalDollarsPerHour) > minDollarsPerHourDifference

	spotPriceChanged := fmt.Sprintf("%v", newSpotPrice) != *launchConfiguration.SpotPrice
	instanceChanged := *launchConfiguration.InstanceType != newInstanceType
	configChanged := spotPriceChanged || instanceChanged

	if anySatisfyConstraints && (scaleMemory || (passesDollarDifference && configChanged)) {
		UpdateLaunchConfiguration(autoScalingGroup, launchConfiguration, allLaunchConfigurations, spotConfig,
			minActualDollarsPerHour, newSpotPrice, newInstanceType, monitor)
		return true
	}
	return false
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
			updated = CheckAndUpdate(sess, spotConfig, priceList, podSummary, clientset, monitor)
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
