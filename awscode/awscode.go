package awscode

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/spf13/cobra"
)

func GetNewestFeed(svc *s3.S3, bucketName string, feedPrefix string) (string, error) {
	params := &s3.ListObjectsInput{
		Bucket: aws.String(bucketName),
		Prefix: aws.String(feedPrefix),
	}

	resp, _ := svc.ListObjects(params)
	baseTime := "2006-01-02-15"
	newest := ""
	newestTime, _ := time.Parse(baseTime, "2017-01-01-00")

	r, _ := regexp.Compile("spot-data-feeds/[\\d+]+\\.([\\d\\-]+)\\.")

	for _, key := range resp.Contents {
		timeMatch := r.FindStringSubmatch(*key.Key)
		if len(timeMatch) == 0 {
			continue
		}
		rawTime := timeMatch[0]
		parsedTime, _ := time.Parse(baseTime, rawTime)
		if parsedTime.After(newestTime) {
			newestTime = parsedTime
			newest = *key.Key
		}
		fmt.Printf("RawDate: %v | ParsedDate: %v", rawTime, parsedTime)
	}
	if newest == "" {
		return "", errors.New("There are no keys under the 'spot-data-feeds/' prefix that conform" +
			"to standard spot pricing naming conventions...")
	}

	return newest, nil

}

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

func ReadDetails(filename string) []SpotDetails {
	csvFile, err := os.Open(filename)

	if err != nil {
		fmt.Println(err)
	}

	defer csvFile.Close()

	reader := csv.NewReader(csvFile)

	reader.Comma = '\t' // Use tab-delimited instead of comma <---- here!

	reader.FieldsPerRecord = -1

	csvData, err := reader.ReadAll()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var oneRecord SpotDetails
	var allRecords []SpotDetails

	for _, each := range csvData {
		oneRecord.Timestamp = each[0]
		oneRecord.UsageType = each[1]
		oneRecord.Operation = each[2]
		oneRecord.InstanceID = each[3]
		oneRecord.MyBidID = each[4]
		oneRecord.MyMaxPrice = each[5]
		oneRecord.MarketPrice = each[6]
		oneRecord.Charge = each[7]
		oneRecord.Version = each[8]
		allRecords = append(allRecords, oneRecord)
	}
	return allRecords

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
		// Filters: []*ec2.Filter{
		// 	{ // Required
		// 		Name: aws.String("String"),
		// 		Values: []*string{
		// 			aws.String("String"), // Required
		// 			// More values...
		// 		},
		// 	},
		// 	// More values...
		// },
		InstanceTypes: awsInstanceTypes,
		MaxResults:    aws.Int64(1000),
		// NextToken:     aws.String("String"),
		ProductDescriptions: []*string{
			aws.String("Linux/UNIX"), // Required
			// More values...
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
	MaxCV                        float64 `yaml:"maxCV"`
	MinGB                        float64 `yaml:"minGB"`
	MaxDollarsPerGB              float64 `yaml:"maxDollarsPerGB"`
	MaxDollarsPerCPU             float64 `yaml:"maxDollarsPerCPU"`
	AutoscalingGroupName         string  `yaml:"autoscalingGroupName"`
	MaxAutoscalingNodes          int     `yaml:"maxAutoscalingNodes"`
	HistoricalHours              float64 `yaml:"historicalHours"`
	RegionName                   string  `yaml:"regionName"`
	MaxTotalDollarsPerHour       float64 `yaml:"maxTotalDollarsPerHour"`
	MinMarkupPercentage          float64 `yaml:"minMarkupPercentage"`
	MinPriceDifferencePercentage float64 `yaml:"minPriceDifferencePercentage"`
	MaxPodKills                  int     `yaml:"maxPodKills"`
	MemoryBufferPercentage       float64 `yaml:"memoryBufferPercentage"`
	UpdateIntervalSeconds        float64 `yaml:"updateIntervalSeconds"`
}

func GetSpotConfigFromCommand(cmd *cobra.Command) SpotConfig {
	maxCV, _ := cmd.PersistentFlags().GetFloat64("maxCV")
	minGB, _ := cmd.PersistentFlags().GetFloat64("minGB")
	maxDollarsPerGB, _ := cmd.PersistentFlags().GetFloat64("maxDollarsPerGB")
	maxDollarsPerCPU, _ := cmd.PersistentFlags().GetFloat64("maxDollarsPerCPU")
	autoscalingGroupName, _ := cmd.PersistentFlags().GetString("autoscalingGroupName")
	maxAutoscalingNodes, _ := cmd.PersistentFlags().GetInt("maxAutoscalingNodes")
	historicalHours, _ := cmd.PersistentFlags().GetFloat64("historicalHours")
	regionName, _ := cmd.PersistentFlags().GetString("regionName")
	maxTotalDollarsPerHour, _ := cmd.PersistentFlags().GetFloat64("maxTotalDollarsPerHour")
	minMarkupPercentage, _ := cmd.PersistentFlags().GetFloat64("minMarkupPercentage")
	minPriceDifferencePercentage, _ := cmd.PersistentFlags().GetFloat64("minPriceDifferencePercentage")
	maxPodKills, _ := cmd.PersistentFlags().GetInt("maxPodKills")
	memoryBufferPercentage, _ := cmd.PersistentFlags().GetFloat64("memoryBufferPercentage")
	updateIntervalSeconds, _ := cmd.PersistentFlags().GetFloat64("updateIntervalSeconds")

	return SpotConfig{
		MaxCV:                        maxCV,
		MinGB:                        minGB,
		MaxDollarsPerGB:              maxDollarsPerGB,
		MaxDollarsPerCPU:             maxDollarsPerCPU,
		AutoscalingGroupName:         autoscalingGroupName,
		MaxAutoscalingNodes:          maxAutoscalingNodes,
		HistoricalHours:              historicalHours,
		RegionName:                   regionName,
		MaxTotalDollarsPerHour:       maxTotalDollarsPerHour,
		MinMarkupPercentage:          minMarkupPercentage,
		MinPriceDifferencePercentage: minPriceDifferencePercentage,
		MaxPodKills:                  maxPodKills,
		MemoryBufferPercentage:       memoryBufferPercentage,
		UpdateIntervalSeconds:        updateIntervalSeconds}
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

func GetLaunchConfiguration(sess *session.Session, launchConfigurationName string) *autoscaling.LaunchConfiguration {
	autoscaling_svc := autoscaling.New(sess)

	params := &autoscaling.DescribeLaunchConfigurationsInput{
		LaunchConfigurationNames: []*string{aws.String(launchConfigurationName)},
		MaxRecords:               aws.Int64(10),
	}
	resp, err := autoscaling_svc.DescribeLaunchConfigurations(params)
	if err != nil {
		panic(err)
	}
	if len(resp.LaunchConfigurations) != 1 {
		panic(fmt.Sprintf(
			"You should not have more or less than 1 matched launchConfiguration to launchConfiguration name '%v'.  You have %v",
			launchConfigurationName, len(resp.LaunchConfigurations)))
	}
	return resp.LaunchConfigurations[0]

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
