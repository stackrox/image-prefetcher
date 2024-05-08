package main

import (
	_ "embed"
	"log"
	"os"
	"text/template"
)

type settings struct {
	Name            string
	Image           string
	Version         string
	Secret          string
	IsCRIO          bool
	NeedsPrivileged bool
}

const imageRepo = "quay.io/stackrox-io/image-prefetcher"

//go:embed deployment.yaml.gotpl
var daemonSetTemplate string

func main() {
	if len(os.Args) < 4 {
		println("Usage:", os.Args[0], "<name> <version> vanilla|ocp [secret]")
		os.Exit(1)
	}
	name := os.Args[1]
	version := os.Args[2]
	isOcp := os.Args[3] == "ocp"
	secret := ""
	if len(os.Args) > 4 {
		secret = os.Args[4]
	}

	s := settings{
		Name:            name,
		Image:           imageRepo,
		Version:         version,
		Secret:          secret,
		IsCRIO:          isOcp,
		NeedsPrivileged: isOcp,
	}
	tmpl := template.Must(template.New("deployment").Parse(daemonSetTemplate))
	if err := tmpl.Execute(os.Stdout, s); err != nil {
		log.Fatal(err)
	}
}
