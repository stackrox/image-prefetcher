package main

import (
	"fmt"
	"log"
	"slices"
)

type k8sFlavorType string

func (k *k8sFlavorType) UnmarshalText(text []byte) error {
	if slices.Contains(allFlavors, string(text)) {
		*k = k8sFlavorType(text)
		return nil
	}
	return fmt.Errorf("unknown k8s flavor %q", text)
}

func (k *k8sFlavorType) MarshalText() (text []byte, err error) {
	return []byte(*k), nil
}

func flavor(flavor string) *k8sFlavorType {
	var f k8sFlavorType
	if err := f.UnmarshalText([]byte(flavor)); err != nil {
		log.Fatal(err)
	}
	return &f
}
