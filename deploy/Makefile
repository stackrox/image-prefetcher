BASE_PATH ?= $(CURDIR)

include $(BASE_PATH)/../make/gotools.mk

$(call go-tool, KUBECONFORM, github.com/yannh/kubeconform/cmd/kubeconform, tools)

.PHONY: kubeconform
kubeconform: $(KUBECONFORM)
	$(KUBECONFORM) --verbose $(MANIFESTS)
