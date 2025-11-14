package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"
)

type settings struct {
	Name            string
	Namespace       string
	Image           string
	Version         string
	Secret          string
	IsCRIO          bool
	NeedsPrivileged bool
	CollectMetrics  bool
}

const (
	vanillaFlavor = "vanilla"
	ocpFlavor     = "ocp"
)

var allFlavors = []string{vanillaFlavor, ocpFlavor}

const imageRepo = "quay.io/stackrox-io/image-prefetcher"

//go:embed deployment.yaml.gotpl
var deploymentTemplate string

var (
	version        string
	namespace      string
	k8sFlavor      k8sFlavorType
	secret         string
	collectMetrics bool
)

func init() {
	flag.StringVar(&version, "version", "v0.3.0", "Version of image prefetcher OCI image.")
	flag.StringVar(&namespace, "namespace", "default", "Namespace where the image prefetcher will be deployed.")
	flag.TextVar(&k8sFlavor, "k8s-flavor", flavor(vanillaFlavor), fmt.Sprintf("Kubernetes flavor. Accepted values: %s", strings.Join(allFlavors, ",")))
	flag.StringVar(&secret, "secret", "", "Kubernetes image pull Secret to use when pulling.")
	flag.BoolVar(&collectMetrics, "collect-metrics", false, "Whether to collect and expose image pull metrics.")
}

func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		println("Usage:", os.Args[0], "[ FLAGS ] <name>")
		println("Note: name MUST come AFTER flags, if any.")
		os.Exit(1)
	}
	name := flag.Arg(0)
	isOcp := k8sFlavor == ocpFlavor

	s := settings{
		Name:            name,
		Namespace:       namespace,
		Image:           imageRepo,
		Version:         version,
		Secret:          secret,
		IsCRIO:          isOcp,
		NeedsPrivileged: isOcp,
		CollectMetrics:  collectMetrics,
	}
	tmpl := template.Must(template.New("deployment").Parse(deploymentTemplate))
	if err := tmpl.Execute(os.Stdout, s); err != nil {
		log.Fatal(err)
	}
}
