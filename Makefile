DAGGO ?= go

DAGMOD := github.com/ribasushi/DAGger
DAGMOD_EXTRA := github.com/ipfs/go-qringbuf

DAGTAG_NOSANCHECKS := nosanchecks_DAGger nosanchecks_qringbuf

### !!! Needs to be set when using padfinder_rure
###     See https://github.com/BurntSushi/rure-go#install
# export CGO_LDFLAGS=-L$(HOME)/devel/regex/target/release
# DAGTAG_PADFINDER_TYPE=padfinder_rure

CROSSBUILD := $(addprefix crossbuild-, \
	linux/386 linux/amd64 linux/arm64 linux/ppc64 linux/mips \
	darwin/amd64 darwin/arm64 \
	freebsd/arm openbsd/386 dragonfly/amd64 \
	windows/386 windows/amd64 \
)

.PHONY: $(MAKECMDGOALS) $(CROSSBUILD)

# trimpath is only available on go 1.13+
GO_DOES_NOT_KNOW_TRIMPATH := $(shell $(DAGGO) version | grep -o 1\.1[12]\.)
ifeq ($(GO_DOES_NOT_KNOW_TRIMPATH),)
	TRIMPATH := -trimpath
else
	TRIMDIRS := $(shell pwd):$(shell $(DAGGO) env GOROOT):$(shell $(DAGGO) env GOPATH)
	TRIMPATH := -gcflags=all="-trimpath=$(TRIMDIRS)" -asmflags=all="-trimpath=$(TRIMDIRS)"
endif
DAGLD_STRIP := $(TRIMPATH) -ldflags=all="-s -w -buildid="

DAGGC_NOBOUNDCHECKS := -gcflags=all="-B -C"

build: check-go-version
	@mkdir -p bin/
	$(DAGGO) build \
		-tags "$(DAGTAG_PADFINDER_TYPE)" \
		-o bin/stream-dagger ./cmd/stream-dagger

	$(DAGGO) build \
		-tags "$(DAGTAG_PADFINDER_TYPE)" \
		-o bin/stream-repack-multipart ./cmd/stream-repack-multipart

build-all: build $(CROSSBUILD)
	@mkdir -p tmp/pprof

	$(DAGGO) build \
		-tags "$(DAGTAG_PADFINDER_TYPE) $(DAGTAG_NOSANCHECKS)" \
		$(DAGGC_NOBOUNDCHECKS) \
		-o bin/stream-dagger-nochecks ./cmd/stream-dagger

	$(DAGGO) build \
		-tags "profile $(DAGTAG_PADFINDER_TYPE)" "-ldflags=-X $(DAGMOD)/internal/util/profiler.profileOutDir=$(shell pwd)/tmp/pprof" \
		"-gcflags=all=-l" \
		-o bin/stream-dagger-writepprof ./cmd/stream-dagger

	$(DAGGO) build \
		-tags "profile $(DAGTAG_PADFINDER_TYPE) $(DAGTAG_NOSANCHECKS)" "-ldflags=-X $(DAGMOD)/internal/util/profiler.profileOutDir=$(shell pwd)/tmp/pprof" \
		$(DAGGC_NOBOUNDCHECKS) \
		"-gcflags=all=-l" \
		-o bin/stream-dagger-writepprof-nochecks ./cmd/stream-dagger

$(CROSSBUILD): %:
	@mkdir -p bin/crossbuild

	GOOS=$(patsubst crossbuild-%/,%,$(dir $*)) GOARCH=$(notdir $*) \
		$(DAGGO) build \
		-tags "$(DAGTAG_PADFINDER_TYPE)" \
		$(DAGLD_STRIP) \
		-o bin/crossbuild/$(patsubst crossbuild-%/,%,$(dir $*))-$(notdir $*)_stream-dagger ./cmd/stream-dagger

	GOOS=$(patsubst crossbuild-%/,%,$(dir $*)) GOARCH=$(notdir $*) \
		$(DAGGO) build \
		-tags "$(DAGTAG_PADFINDER_TYPE)" \
		$(DAGLD_STRIP) \
		-o bin/crossbuild/$(patsubst crossbuild-%/,%,$(dir $*))-$(notdir $*)_stream-repack-multipart ./cmd/stream-repack-multipart


test: build build-maint $(CROSSBUILD)
	@# anything above 32 and we blow through > 256 open file handles
	$(DAGGO) test -tags "$(DAGTAG_PADFINDER_TYPE)" -timeout=0 -parallel=32 -count=1 -failfast ./...

build-maint:
	mkdir -p tmp/maintbin
	# build the maint tools without boundchecks to speed things up
	$(DAGGO) build -o tmp/maintbin/dezstd ./maint/src/dezstd


analyze-all:
	$(DAGGO) build \
		-tags "$(DAGTAG_PADFINDER_TYPE)" \
		$(addsuffix /...="-m -m",$(addprefix -gcflags=,$(DAGMOD) $(DAGMOD_EXTRA))) \
		-o /dev/null ./cmd/stream-dagger 2>&1 | ( [ -t 1 ] && less -SRI || cat )

analyze-all-nochecks:
	$(DAGGO) build \
		-tags "$(DAGTAG_PADFINDER_TYPE) $(DAGTAG_NOSANCHECKS)" \
		$(DAGGC_NOBOUNDCHECKS) \
		$(addsuffix /...="-m -m",$(addprefix -gcflags=,$(DAGMOD) $(DAGMOD_EXTRA))) \
		-o /dev/null ./cmd/stream-dagger 2>&1 | ( [ -t 1 ] && less -SRI || cat )

analyze-bound-checks:
	$(DAGGO) build \
		-tags "$(DAGTAG_PADFINDER_TYPE)" \
		$(addsuffix /...="-d=ssa/check_bce/debug=1",$(addprefix -gcflags=,$(DAGMOD) $(DAGMOD_EXTRA))) \
		-o /dev/null ./cmd/stream-dagger 2>&1 | ( [ -t 1 ] && less -SRI || cat )


serve-latest-pprof-cpu:
	$(DAGGO) tool pprof -http=:9090 tmp/pprof/latest_cpu.prof

serve-latest-pprof-allocs:
	$(DAGGO) tool pprof -http=:9090 -alloc_objects tmp/pprof/latest_allocs.prof


check-go-version:
	@# stop early if go 1.11+ isn't available
	$(DAGGO) run ./maint/src/vercheck/main.go
