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
	k8sFlavor      k8sFlavorType
	secret         string
	collectMetrics bool
)

func init() {
	flag.StringVar(&version, "version", "v0.1.0", "Version of image prefetcher OCI image.")
	flag.TextVar(&k8sFlavor, "k8s-flavor", flavor(vanillaFlavor), fmt.Sprintf("Kubernetes flavor. Accepted values: %s", strings.Join(allFlavors, ",")))
	flag.StringVar(&secret, "secret", "", "Kubernetes image pull Secret to use when pulling.")
	flag.BoolVar(&collectMetrics, "collect-metrics", false, "Whether to collect and expose image pull metrics.")
}

func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		println("Usage:", os.Args[0], "[ FLAGS ] <name>")
		os.Exit(1)
	}
	name := flag.Arg(0)
	isOcp := k8sFlavor == ocpFlavor

	s := settings{
		Name:            name,
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
