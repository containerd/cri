GO ?= go
EPOCH_TEST_COMMIT ?= f2925f58acc259c4b894353f5fc404bdeb40028e
PROJECT := github.com/kubernetes-incubator/cri-containerd
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
GIT_BRANCH_CLEAN := $(shell echo $(GIT_BRANCH) | sed -e "s/[^[:alnum:]]/-/g")
PREFIX ?= ${DESTDIR}/usr/local
BINDIR ?= ${PREFIX}/bin

all: binaries

default: help

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'install' - Install binaries to system locations"
	@echo " * 'binaries' - Build cri-containerd"
	@echo " * 'clean' - Clean artifacts"
	@echo " * 'lint' - Execute the source code linter"
	@echo " * 'gofmt' - Verify the source code gofmt"

.PHONY: check-gopath

check-gopath:
ifndef GOPATH
	$(error GOPATH is not set)
endif

lint: check-gopath
	@echo "checking lint"
	@./.tool/lint

gofmt:
	@./hack/verify-gofmt.sh

cri-containerd: check-gopath
	$(GO) build -o $@ \
		$(PROJECT)/cmd/cri-containerd

clean:
	find . -name \*~ -delete
	find . -name \#\* -delete
	rm -f cri-containerd

binaries: cri-containerd

install: check-gopath
	install -D -m 755 cri-containerd $(BINDIR)/cri-containerd

uninstall:
	rm -f $(BINDIR)/cri-containerd

.PHONY: .gitvalidation
# When this is running in travis, it will only check the travis commit range
.gitvalidation: check-gopath
ifeq ($(TRAVIS),true)
	git-validation -q -run DCO,short-subject
else
	git-validation -v -run DCO,short-subject -range $(EPOCH_TEST_COMMIT)..HEAD
endif

.PHONY: install.tools

install.tools: .install.gitvalidation .install.gometalinter .install.md2man

.install.gitvalidation:
	go get -u github.com/vbatts/git-validation

.install.gometalinter:
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install

.install.md2man:
	go get -u github.com/cpuguy83/go-md2man

.PHONY: \
	binaries \
	clean \
	default \
	gofmt \
	help \
	install \
	lint \
	uninstall
