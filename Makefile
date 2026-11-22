SUBDIR :=
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)

.PHONY: all clean test run build upgrade help $(SUBDIR)

all: $(SUBDIR) 		# default action
	@[ -f .git/hooks/pre-commit ] || pre-commit install --install-hooks
	@git config commit.template .git-commit-template

clean: $(SUBDIR)	# clean-up environment
	@find . -name '*.sw[po]' -delete
	rm -rf bin

test:				# run test
	go vet ./...
	go test ./...

run:				# run in the local environment

build:				# build the binary/library
	go build -ldflags "-X github.com/cmj0121/loci/internal/loci.Version=$(VERSION)" -o bin/loci ./cmd/loci

upgrade:			# upgrade all the necessary packages
	pre-commit autoupdate
	go get -u ./... && go mod tidy

help:				# show this message
	@printf "Usage: make [OPTION]\n"
	@printf "\n"
	@perl -nle 'print $$& if m{^[\w-]+:.*?#.*$$}' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?#"} {printf "    %-18s %s\n", $$1, $$2}'

$(SUBDIR):
	$(MAKE) -C $@ $(MAKECMDGOALS)
