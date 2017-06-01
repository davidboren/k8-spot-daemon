package pricing

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/montanaflynn/stats"
	// "github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/davidboren/k8-spot-daemon/awscode"
	instanceConfig "github.com/davidboren/k8-spot-daemon/config"
)

type FullSummary struct {
	Name        string
	Price       float64
	CoefVar     float64
	StdDev      float64
	Cpus        float64
	Mem         float64
	PricePerCPU float64
	PricePerGB  float64
}

type InstanceDetails struct {
	Name string  `yaml:"name"`
	Mem  float64 `yaml:"memory"`
	Cpus float64 `yaml:"cpus"`
}

func ReadDetails() map[string]InstanceDetails {
	machines, _ := instanceConfig.Asset("config/machines.yaml")

	detailList := make([]InstanceDetails, 0)
	detailMap := make(map[string]InstanceDetails)

	yaml.Unmarshal(machines, &detailList)

	for _, each := range detailList {
		detailMap[each.Name] = each
	}
	return detailMap
}

func TimeWeight(now time.Time, timeStamp time.Time) float64 {
	hoursAgo := now.Sub(timeStamp).Hours()
	return 1.0 / (0.2 + hoursAgo)
}

func DescribePricing(sess *session.Session, spotConfig awscode.SpotConfig) []FullSummary {
	instanceDetails := ReadDetails()
	bigInstanceTypes := map[string]InstanceDetails{}
	for _, each := range instanceDetails {
		if each.Mem >= float64(spotConfig.MinGB) {
			bigInstanceTypes[each.Name] = each
		}
	}
	regionNames := []string{spotConfig.RegionName}

	avgList := CompileAverages(sess, bigInstanceTypes, regionNames,
		time.Duration(spotConfig.HistoricalHours), TimeWeight)

	sort.Sort(ByPricePerGB(avgList))
	fmt.Printf("Averaged Pricing Data for last '%v' hours: \n", spotConfig.HistoricalHours)
	for _, obj := range avgList {
		if obj.CoefVar < spotConfig.MaxCV {
			fmt.Printf("    %12v || Price: %7.3f | GB: %9.4f | Cpus: %3v | Cpus/GB: %0.3f | Price/GB: %8.4f | Price/Cpu: %3.4f | Coef of Var: %8.4f\n",
				obj.Name,
				obj.Price,
				obj.Mem,
				obj.Cpus,
				float64(obj.Cpus)/float64(obj.Mem),
				obj.PricePerGB,
				obj.PricePerCPU,
				obj.CoefVar)
		}
	}
	return avgList
}

// ByPrice implements sort.Interface for []PriceSummary based on
// the Price field.
type ByPrice []FullSummary

func (a ByPrice) Len() int           { return len(a) }
func (a ByPrice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByPrice) Less(i, j int) bool { return a[i].Price < a[j].Price }

// ByPrice implements sort.Interface for []PriceSummary based on
// the Price field.
type ByPricePerGB []FullSummary

func (a ByPricePerGB) Len() int           { return len(a) }
func (a ByPricePerGB) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByPricePerGB) Less(i, j int) bool { return a[i].PricePerGB < a[j].PricePerGB }

func Volatility(prices []float64) (float64, float64) {
	priceMean, _ := stats.Mean(prices)
	priceSTD, _ := stats.StandardDeviation(prices)
	return priceSTD / priceMean, priceSTD
}

func CompileAverages(sess *session.Session, instanceDetails map[string]InstanceDetails,
	regionNames []string, historicalHours time.Duration,
	TimeWeight func(time.Time, time.Time) float64) []FullSummary {

	instanceTypes := []string{}
	for _, obj := range instanceDetails {
		instanceTypes = append(instanceTypes, obj.Name)
	}
	// fmt.Printf("instanceTypes: %v\n", instanceTypes)
	if len(instanceTypes) == 0 {
		panic("You have no instanceTypes...")
	}
	priceMap := awscode.GetSpotPrices(sess, instanceTypes, regionNames, historicalHours)
	sumList := []FullSummary{}
	now := time.Now()
	for _, intype := range instanceTypes {
		// fmt.Printf("History for %v: %v\n\n", intype, priceMap[intype])
		weightSum := 0.0
		timestamps := []time.Time{}
		prices := []float64{}
		weights := []float64{}
		for _, spotPrice := range priceMap[intype] {
			curPrice, _ := strconv.ParseFloat(*spotPrice.SpotPrice, 64)
			weight := TimeWeight(now, *spotPrice.Timestamp)
			weightSum += weight
			weights = append(weights, weight)
			timestamps = append(timestamps, *spotPrice.Timestamp)
			prices = append(prices, curPrice)
		}
		priceSum := 0.0
		for i, _ := range priceMap[intype] {
			curPrice := prices[i]
			weight := weights[i]
			priceSum += curPrice * weight / weightSum
			// fmt.Printf("Price for %v: %v | Weight: %v Total Weight %v\n\n", intype, curPrice, weight, weightSum)
		}
		inDet := instanceDetails[intype]
		cv, std := Volatility(prices)
		sumList = append(
			sumList,
			FullSummary{
				Name:        intype,
				Price:       priceSum,
				CoefVar:     cv,
				StdDev:      std,
				Cpus:        inDet.Cpus,
				Mem:         inDet.Mem,
				PricePerCPU: priceSum / float64(inDet.Cpus),
				PricePerGB:  priceSum / inDet.Mem})
	}
	return sumList
}
