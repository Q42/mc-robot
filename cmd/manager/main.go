package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"

	"q42/mc-robot/pkg/apis"
	"q42/mc-robot/pkg/controller"
	"q42/mc-robot/pkg/datasource"
	"q42/mc-robot/pkg/datasourcegoogle"
	"q42/mc-robot/version"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	kubemetrics "github.com/operator-framework/operator-sdk/pkg/kube-metrics"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/metrics"
	"github.com/operator-framework/operator-sdk/pkg/restmapper"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	// Logging documentation: https://github.com/operator-framework/operator-sdk/blob/master/doc/user/logging.md
	"github.com/blendle/zapdriver"
	"github.com/go-logr/zapr"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost         = "0.0.0.0"
	metricsPort         = 8383
	operatorMetricsPort = 8686
)
var log = logf.Log.WithName("cmd")

func printVersion() {
	log.Info(fmt.Sprintf("Operator Version: %s", version.Version))
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
}

func init() {
	// Overridable defaults
	pflag.IntVar(&metricsPort, "metricsPort", metricsPort, "Which port to expose metrics")
	pflag.IntVar(&operatorMetricsPort, "operatorMetricsPort", operatorMetricsPort, "Which port to expose operator metrics")
}

func main() {
	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	pflag.CommandLine.AddFlagSet(zap.FlagSet())

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()

	// Use a zap logr.Logger implementation compatible with
	// StackDriver (JSON & correct fields).
	// This logger will be propagated through the whole operator,
	// generating uniform and structured logs.
	stackDriverOptimizedLogger, err := zapdriver.NewProductionWithCore(zapdriver.WrapCore(
		zapdriver.ReportAllErrors(true),
		zapdriver.ServiceName("mc-robot"),
	))
	if err != nil {
		log.Error(err, "Failed to create logger")
		os.Exit(1)
	}
	logf.SetLogger(zapr.NewLogger(stackDriverOptimizedLogger))

	printVersion()

	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		log.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, err.Error())
		os.Exit(1)
	}

	ctx := context.TODO()
	// Become the leader before proceeding
	err = leader.Become(ctx, "mc-robot-lock")
	if err != nil {
		log.Error(err, err.Error())
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		Namespace:          namespace,
		MapperProvider:     restmapper.NewDynamicRESTMapper,
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
	})
	if err != nil {
		log.Error(err, err.Error())
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, err.Error())
		os.Exit(1)
	}

	// Setup datasource
	ds := datasource.New(datasourcegoogle.Google)

	// Setup all Controllers
	if err := controller.AddToManager(mgr, ds); err != nil {
		log.Error(err, err.Error())
		os.Exit(1)
	}

	if err = serveCRMetrics(cfg); err != nil {
		log.Info("Could not generate and serve custom resource metrics", "error", err.Error())
	}

	// Add to the below struct any other metrics ports you want to expose.
	servicePorts := []v1.ServicePort{
		{Port: int32(metricsPort), Name: metrics.OperatorPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: int32(metricsPort)}},
		{Port: int32(operatorMetricsPort), Name: metrics.CRPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: int32(operatorMetricsPort)}},
	}
	// Create Service object to expose the metrics port(s).
	service, err := metrics.CreateMetricsService(ctx, cfg, servicePorts)
	if err != nil {
		log.Info("Could not create metrics Service", "error", err.Error())
	}

	// CreateServiceMonitors will automatically create the prometheus-operator ServiceMonitor resources
	// necessary to configure Prometheus to scrape metrics from this operator.
	services := []*v1.Service{service}
	_, err = metrics.CreateServiceMonitors(cfg, namespace, services)
	if err != nil {
		log.Info("Could not create ServiceMonitor object", "error", err.Error())
		// If this operator is deployed to a cluster without the prometheus-operator running, it will return
		// ErrServiceMonitorNotPresent, which can be used to safely skip ServiceMonitor creation.
		if err == metrics.ErrServiceMonitorNotPresent {
			log.Info("Install prometheus-operator in your cluster to create ServiceMonitor objects", "error", err.Error())
		}
	}

	log.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}

// serveCRMetrics gets the Operator/CustomResource GVKs and generates metrics based on those types.
// It serves those metrics on "http://metricsHost:operatorMetricsPort".
func serveCRMetrics(cfg *rest.Config) error {
	// Below function returns filtered operator/CustomResource specific GVKs.
	// For more control override the below GVK list with your own custom logic.
	filteredGVK, err := k8sutil.GetGVKsFromAddToScheme(apis.AddToScheme)
	if err != nil {
		return err
	}
	// Get the namespace the operator is currently deployed in.
	operatorNs, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return err
	}
	// To generate metrics in other namespaces, add the values below.
	ns := []string{operatorNs}
	// Generate and serve custom resource specific metrics.
	err = kubemetrics.GenerateAndServeCRMetrics(cfg, ns, filteredGVK, metricsHost, int32(operatorMetricsPort))
	if err != nil {
		return err
	}
	return nil
}
