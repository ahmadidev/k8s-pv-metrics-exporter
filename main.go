package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	getFolderSize "github.com/markthree/go-get-folder-size/src"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	FlagKubeconfig = "kubeconfig"
)

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func loadConfig(kubeconfig string) (*rest.Config, error) {
	if len(kubeconfig) > 0 {
		fmt.Printf("Using kubeconfig %s\n", kubeconfig)
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	kubeconfigPath := os.Getenv(clientcmd.RecommendedConfigPathEnvVar)
	if len(kubeconfigPath) > 0 {
		envVarFiles := filepath.SplitList(kubeconfigPath)
		for _, f := range envVarFiles {
			if _, err := os.Stat(f); err == nil {
				fmt.Printf("Using RecommendedConfigPathEnvVar %s\n", clientcmd.RecommendedConfigPathEnvVar)
				return clientcmd.BuildConfigFromFlags("", f)
			}
		}
	}

	if c, err := rest.InClusterConfig(); err == nil {
		fmt.Println("Using InClusterConfig")
		return c, nil
	}

	kubeconfig = filepath.Join(homeDir(), clientcmd.RecommendedHomeDir, clientcmd.RecommendedFileName)
	fmt.Printf("Using homeDir %s", kubeconfig)
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func StartCmd() cli.Command {
	return cli.Command{
		Name: "start",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  FlagKubeconfig,
				Usage: "Paths to a kubeconfig. Only required when it is out-of-cluster.",
				Value: "",
			},
		},
		Action: func(c *cli.Context) {
			if err := startDaemon(c); err != nil {
				logrus.Fatalf("Error starting daemon: %v", err)
			}
		},
	}
}

func startDaemon(c *cli.Context) error {
	sleepSeconds, err := strconv.Atoi(os.Getenv("SLEEP_SECONDS"))
	if err != nil {
		logrus.Fatalf("Parsing SLEEP_SECONDS error: %v", err)
	}

	fmt.Println("startDaemon")

	config, err := loadConfig(c.String(FlagKubeconfig))
	if err != nil {
		logrus.Fatalf("loadConfig error: %v", err)
	}

	kubeClient, err := clientset.NewForConfig(config)
	if err != nil {
		logrus.Fatalf("NewForConfig error: %v", err)
	}

	pv_labels := []string{"pv_name", "pvc_name", "pvc_namespace"}
	pv_total_gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "k8s_pv_total_bytes",
		Help: "Kubernetes Persistent Volume Total Bytes",
	}, pv_labels)
	pv_used_gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "k8s_pv_used_bytes",
		Help: "Kubernetes Persistent Volume Used Bytes",
	}, pv_labels)

	// Register the metric with the default Prometheus registry
	prometheus.MustRegister(pv_total_gauge)
	prometheus.MustRegister(pv_used_gauge)

	go func() {
		for {
			fmt.Println("Updating PVs usage:")

			// Use the clientset to retrieve a list of PVs
			// TODO: Check Bound PVs or all?
			// TODO: Remove removed pvc from metrics
			// TODO: Filter for local-path storageClass.
			//		field selector is not available for storageClass, we may use annotation selector for: pv.kubernetes.io/provisioned-by: rancher.io/local-path
			//		or filter on loop for spec.storageClass

			pvList, err := kubeClient.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{})
			if err != nil {
				fmt.Printf("Failed to retrieve PV list: %v\n", err)
				os.Exit(1)
			}

			for _, pv := range pvList.Items {
				if pv.Spec.StorageClassName != "local-path" || pv.Status.Phase != "Bound" {
					continue
				}

				capacity, ok := pv.Spec.Capacity[corev1.ResourceName(corev1.ResourceStorage)]
				if !ok {
					fmt.Printf("Failed to get PV size\n")
					os.Exit(1)
				}
				pvSize := capacity.Value()

				size, err := getFolderSize.Parallel(pv.Spec.HostPath.Path)
				if err != nil {
					fmt.Printf("Failed to find disk usage for path %s: %v\n", pv.Spec.HostPath.Path, err)
					os.Exit(1)
				}

				fmt.Printf("Name: %s - Size: %d - Used: %d\n", pv.Name, pvSize, size)

				// Set the value of the metric
				pv_total_gauge.With(prometheus.Labels{"pv_name": pv.Name, "pvc_name": pv.Spec.ClaimRef.Name, "pvc_namespace": pv.Spec.ClaimRef.Namespace}).Set(float64(pvSize))
				pv_used_gauge.With(prometheus.Labels{"pv_name": pv.Name, "pvc_name": pv.Spec.ClaimRef.Name, "pvc_namespace": pv.Spec.ClaimRef.Namespace}).Set(float64(size))
			}

			fmt.Printf("Sleeping for %d seconds before new update...\n", sleepSeconds)
			time.Sleep(time.Duration(sleepSeconds) * time.Second)
		}
	}()

	// Start an HTTP server to expose the metrics
	http.Handle("/metrics", promhttp.Handler())
	fmt.Println("Starting Prometheus exporter on port 9100...")
	log.Fatal(http.ListenAndServe(":9100", nil))

	return nil
}

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	a := cli.NewApp()

	a.Commands = []cli.Command{
		StartCmd(),
	}

	if err := a.Run(os.Args); err != nil {
		fmt.Println("Run error...")
		logrus.Fatalf("Critical error: %v", err)
	}
}
