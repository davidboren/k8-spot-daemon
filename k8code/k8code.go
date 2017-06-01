package k8code

import (
	"os/user"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	// "github.com/aws/aws-sdk-go/service/autoscaling"
)

func GetClientSet() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		usr, _ := user.Current()
		dir := usr.HomeDir
		config, err = clientcmd.BuildConfigFromFlags("", filepath.Join(dir, ".kube", "config"))
		if err != nil {
			panic(err.Error())
		}
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}

// func GetAutoscalerDetails(clientset *kubernetes.Clientset, autoscalerDeploymentName string) (int64, string) {
// 	autoscaler_deployment, err := clientset.ExtensionsV1beta1().Deployments("").List(metav1.ListOptions{
// 		LabelSelector: fmt.Sprintf("app=%v", autoscalerDeploymentName)})
// 	if err != nil {
// 		panic(err.Error())
// 	}
// 	for _, deployment := range autoscaler_deployment.Items {
// 		for _, el := range deployment.Spec.Template.Spec.Containers[0].Command {
// 			r, _ := regexp.Compile("--nodes=[\\d+]+:([\\d]+):(.*)scaling(.*)")
// 			nodeConfig := r.FindStringSubmatch(el)
// 			if len(nodeConfig) != 0 {
// 				maxNum, err := strconv.ParseInt(nodeConfig[1], 10, 64)
// 				if err != nil {
// 					panic(err)
// 				}
// 				return maxNum, (nodeConfig[2] + "scaling" + nodeConfig[3])
// 			}
// 		}
// 	}
// 	return -1, ""
// }

func SummarizePods(clientset *kubernetes.Clientset) map[string]float64 {
	pods, _ := clientset.CoreV1().Pods("").List(metav1.ListOptions{})
	var max_mem int64 = 0
	var tot_mem int64 = 0
	var tot_mem_requested int64 = 0
	var tot_running_pods int64 = 0
	for _, pod := range pods.Items {
		q := pod.Spec.Containers[0].Resources.Requests["memory"]
		gb, _ := q.AsInt64()
		// fmt.Printf("Pod '%v' Size: %v\n", pod.Name, float64(gb)/(1024*1024*1000))
		if pod.Namespace != "kube-system" {
			if max_mem < gb {
				max_mem = gb
			}
			if pod.Status.Phase == v1.PodRunning {
				tot_mem += gb
				tot_mem_requested += gb
				tot_running_pods += 1
			} else if pod.Status.Phase == v1.PodPending {
				tot_mem_requested += gb
				// fmt.Printf("Pod '%v' status: '%v'", pod.Name, pod.Status.Phase)
			}
		}
		// tot_mem_requested += gb
		// fmt.Printf("Pod %60v Memory: %15.3f Bytes\n", pod.ObjectMeta.Name, float64(gb))
	}
	return map[string]float64{
		"totalMemoryRequestedGB": float64(tot_mem_requested) / (1024 * 1024 * 1000),
		"totalMemoryUsedGB":      float64(tot_mem) / (1024 * 1024 * 1000),
		"maxMemoryUsedGB":        float64(max_mem) / (1024 * 1024 * 1000),
		"totalRunningPods":       float64(tot_running_pods)}
}
