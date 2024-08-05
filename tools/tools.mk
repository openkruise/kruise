tools.bindir = tools/bin
tools.srcdir = tools/src

ifeq ($(origin GOOS), undefined)
	GOOS := $(shell go env GOOS)
endif
ifeq ($(origin GOARCH), undefined)
	GOARCH := $(shell go env GOARCH)
endif


# `go get`-able things
# ====================
#
tools/kind           = $(tools.bindir)/kind
$(tools.bindir)/%: $(tools.srcdir)/%/pin.go $(tools.srcdir)/%/go.mod
	cd $(<D) && GOOS= GOARCH= go build -o $(abspath $@) $$(sed -En 's,^import _ "(.*)".*,\1,p' pin.go)
