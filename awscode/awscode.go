package awscode

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/spf13/cobra"
)

type SpotDetails struct {
	Timestamp   string
	UsageType   string
	Operation   string
	InstanceID  string
	MyBidID     string
	MyMaxPrice  string
	MarketPrice string
	Charge      string
	Version     string
}

func Map(vs []string, f func(string) *string) []*string {
	vsm := make([]*string, len(vs))
	for i, v := range vs {
		vsm[i] = f(v)
	}
	return vsm
}

func ToAwsString(v string) *string {
	return aws.String(v)
}

type SpotPriceContainer struct {
	Out *ec2.DescribeSpotPriceHistoryOutput
	Err error
}

func DescribeSpotPriceHistory(sess *session.Session, instanceTypes []string,
	availabilityZone string, priceChan chan SpotPriceContainer, startTime *time.Time) {

	svc := ec2.New(sess)

	awsInstanceTypes := Map(instanceTypes, ToAwsString)

	params := &ec2.DescribeSpotPriceHistoryInput{
		AvailabilityZone: aws.String(availabilityZone),
		DryRun:           aws.Bool(false),
		EndTime:          aws.Time(time.Now()),
		InstanceTypes:    awsInstanceTypes,
		MaxResults:       aws.Int64(1000),
		ProductDescriptions: []*string{
			aws.String("Linux/UNIX"), // Required
		},
		StartTime: startTime,
	}
	resp, err := svc.DescribeSpotPriceHistory(params)
	priceChan <- SpotPriceContainer{Out: resp, Err: err}
}

type InstanceDescription struct {
	Name string
	GB   float64
	CPU  int64
}

func GetInstanceTypes(sess *session.Session) {
	svc := ec2.New(sess)

	params := &ec2.DescribeReservedInstancesOfferingsInput{
		DryRun:             aws.Bool(false),
		MaxResults:         aws.Int64(10),
		ProductDescription: aws.String("Linux/UNIX"),
		// Filters: []*ec2.Filter{
		// 	{
		// 		Name:   aws.String("availability-zone"),
		// 		Values: []*string{aws.String(availabilityZone)},
		// 	},
		// },
	}
	resp, _ := svc.DescribeReservedInstancesOfferings(params)
	for _, off := range resp.ReservedInstancesOfferings {
		fmt.Println(*off.InstanceType)
	}
}

type SpotConfig struct {
	MaxCV                        float64
	MinGB                        float64
	MaxDollarsPerGB              float64
	MaxDollarsPerCPU             float64
	AutoScalingGroupName         string
	LaunchConfigurationPrefix    string
	MaxAutoscalingNodes          int
	HistoricalHours              float64
	RegionName                   string
	MaxTotalDollarsPerHour       float64
	MinMarkupPercentage          float64
	MinPriceDifferencePercentage float64
	MaxPodKills                  int
	MemoryBufferPercentage       float64
	UpdateIntervalSeconds        float64
	MinimumTurnoverSeconds       float64
}

func GetSpotConfigFromCommand(cmd *cobra.Command) SpotConfig {
	maxCV, _ := cmd.PersistentFlags().GetFloat64("maxCV")
	minGB, _ := cmd.PersistentFlags().GetFloat64("minGB")
	maxDollarsPerGB, _ := cmd.PersistentFlags().GetFloat64("maxDollarsPerGB")
	maxDollarsPerCPU, _ := cmd.PersistentFlags().GetFloat64("maxDollarsPerCPU")
	autoScalingGroupName, _ := cmd.PersistentFlags().GetString("autoScalingGroupName")
	launchConfigurationPrefix, _ := cmd.PersistentFlags().GetString("launchConfigurationPrefix")
	maxAutoscalingNodes, _ := cmd.PersistentFlags().GetInt("maxAutoscalingNodes")
	historicalHours, _ := cmd.PersistentFlags().GetFloat64("historicalHours")
	regionName, _ := cmd.PersistentFlags().GetString("regionName")
	maxTotalDollarsPerHour, _ := cmd.PersistentFlags().GetFloat64("maxTotalDollarsPerHour")
	minMarkupPercentage, _ := cmd.PersistentFlags().GetFloat64("minMarkupPercentage")
	minPriceDifferencePercentage, _ := cmd.PersistentFlags().GetFloat64("minPriceDifferencePercentage")
	maxPodKills, _ := cmd.PersistentFlags().GetInt("maxPodKills")
	memoryBufferPercentage, _ := cmd.PersistentFlags().GetFloat64("memoryBufferPercentage")
	updateIntervalSeconds, _ := cmd.PersistentFlags().GetFloat64("updateIntervalSeconds")
	minimumTurnoverSeconds, _ := cmd.PersistentFlags().GetFloat64("minimumTurnoverSeconds")

	return SpotConfig{
		MaxCV:                        maxCV,
		MinGB:                        minGB,
		MaxDollarsPerGB:              maxDollarsPerGB,
		MaxDollarsPerCPU:             maxDollarsPerCPU,
		AutoScalingGroupName:         autoScalingGroupName,
		LaunchConfigurationPrefix:    launchConfigurationPrefix,
		MaxAutoscalingNodes:          maxAutoscalingNodes,
		HistoricalHours:              historicalHours,
		RegionName:                   regionName,
		MaxTotalDollarsPerHour:       maxTotalDollarsPerHour,
		MinMarkupPercentage:          minMarkupPercentage,
		MinPriceDifferencePercentage: minPriceDifferencePercentage,
		MaxPodKills:                  maxPodKills,
		MemoryBufferPercentage:       memoryBufferPercentage,
		UpdateIntervalSeconds:        updateIntervalSeconds,
		MinimumTurnoverSeconds:       minimumTurnoverSeconds}
}

func GetAutoscaler(sess *session.Session, autoscalerName string) *autoscaling.Group {

	autoscaling_svc := autoscaling.New(sess)

	params := &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []*string{aws.String(autoscalerName)},
		MaxRecords:            aws.Int64(10),
	}
	resp, err := autoscaling_svc.DescribeAutoScalingGroups(params)
	if err != nil {
		panic(err)
	}

	if len(resp.AutoScalingGroups) != 1 {
		panic(fmt.Sprintf(
			"You should not have more or less than 1 matched autoscaling groups to autoscaler name '%v'.  You have %v",
			autoscalerName, len(resp.AutoScalingGroups)))
	}
	return resp.AutoScalingGroups[0]
}

func GetLaunchConfigurations(sess *session.Session, launchConfigurationPrefix string) []*autoscaling.LaunchConfiguration {
	autoscaling_svc := autoscaling.New(sess)

	params := &autoscaling.DescribeLaunchConfigurationsInput{
		MaxRecords: aws.Int64(100),
	}
	resp, err := autoscaling_svc.DescribeLaunchConfigurations(params)
	if err != nil {
		panic(err)
	}
	fmt.Printf("\nYou have '%v' total launchconfigurations\n", len(resp.LaunchConfigurations))
	var launchConfigurations []*autoscaling.LaunchConfiguration = []*autoscaling.LaunchConfiguration{}
	for _, lc := range resp.LaunchConfigurations {
		if strings.Contains(*lc.LaunchConfigurationName, launchConfigurationPrefix) {
			launchConfigurations = append(launchConfigurations, lc)
		}
	}
	fmt.Printf("\nYou have '%v' launchconfigurations prefixed by '%v'\n", len(launchConfigurations), launchConfigurationPrefix)
	return launchConfigurations

}

func DuplicateLaunchConfiguration(launchConfiguration *autoscaling.LaunchConfiguration) autoscaling.CreateLaunchConfigurationInput {
	return autoscaling.CreateLaunchConfigurationInput{
		AssociatePublicIpAddress: launchConfiguration.AssociatePublicIpAddress,
		BlockDeviceMappings:      launchConfiguration.BlockDeviceMappings,
		ClassicLinkVPCId:         launchConfiguration.ClassicLinkVPCId,
		EbsOptimized:             launchConfiguration.EbsOptimized,
		IamInstanceProfile:       launchConfiguration.IamInstanceProfile,
		ImageId:                  launchConfiguration.ImageId,
		InstanceMonitoring:       launchConfiguration.InstanceMonitoring,
		InstanceType:             launchConfiguration.InstanceType,
		KernelId:                 launchConfiguration.KernelId,
		KeyName:                  launchConfiguration.KeyName,
		LaunchConfigurationName:  launchConfiguration.LaunchConfigurationName,
		PlacementTenancy:         launchConfiguration.PlacementTenancy,
		RamdiskId:                launchConfiguration.RamdiskId,
		SecurityGroups:           launchConfiguration.SecurityGroups,
		SpotPrice:                launchConfiguration.SpotPrice,
		UserData:                 launchConfiguration.UserData,
	}
}

func GetSpotPrices(sess *session.Session, instanceTypes []string,
	regionNames []string, historicalHours time.Duration) map[string][]ec2.SpotPrice {

	ec2_svc := ec2.New(sess)

	awsRegionNames := Map(regionNames, ToAwsString)

	req := ec2.DescribeAvailabilityZonesInput{
		Filters: []*ec2.Filter{{
			Name:   aws.String("region-name"),
			Values: awsRegionNames}}}
	zones, err := ec2_svc.DescribeAvailabilityZones(&req)

	if err != nil {
		panic(err)
	}

	availabilityZones := zones.AvailabilityZones

	if len(availabilityZones) == 0 {
		panic("You have no relevant AvailabilityZones...")
	}

	priceChan := make(chan SpotPriceContainer)
	priceMap := make(map[string][]ec2.SpotPrice)
	startTime := aws.Time(time.Now().Add(-historicalHours))
	for _, instanceType := range instanceTypes {
		for _, zone := range availabilityZones {
			go DescribeSpotPriceHistory(sess, []string{instanceType}, *zone.ZoneName, priceChan, startTime)
		}
	}

	for _, instanceType := range instanceTypes {
		priceMap[instanceType] = make([]ec2.SpotPrice, 0)
	}
	fullCount := len(instanceTypes) * len(availabilityZones)
	count := 0
	for {
		select {
		case priceContainer := <-priceChan:
			for _, spotPrice := range priceContainer.Out.SpotPriceHistory {
				priceMap[*spotPrice.InstanceType] = append(priceMap[*spotPrice.InstanceType], *spotPrice)
			}
			count++
			if fullCount == count {
				return priceMap
			}
		}
	}
}
