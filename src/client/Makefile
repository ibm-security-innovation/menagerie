ifdef V
BUILDFLAGS = -v
else
BUILDFLAGS =
endif

GOBUILD = GO15VENDOREXPERIMENT=1 go build  $(BUILDFLAGS)
GOTEST = GO15VENDOREXPERIMENT=1 go test  $(BUILDFLAGS)
GOGET = GO15VENDOREXPERIMENT=1 go get  $(BUILDFLAGS)

all: usage build

usage:
	@echo "Use one of the following:"
	@echo "  make build (default)"
	@echo "  make build-386"
	@echo "  make build-arm"

build:
	$(GOBUILD) -o menagerie_cli

build-386:
	GOOS=linux GOARCH=386 $(GOBUILD) -o menagerie_cli-386

build-arm:
	GOOS=linux GOARCH=arm $(GOBUILD) -o menagerie_cli-arm

lint:
	$(GOPATH)/bin/golint .

test:
	$(GOTEST) -o menagerie_cli -httptest.serve=127.0.0.1:3012  -log_dir=logs -stderrthreshold=INFO -v=1 -logtostderr=true

get:
	$(GOGET)
