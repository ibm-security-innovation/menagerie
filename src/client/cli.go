package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/golang/glog"
)

func main() {
	// treat this as a command line invocation, so read the input from the command line
	var request, response, engine, config string
	var help bool

	flag.BoolVar(&help, "h", false, "help")
	flag.StringVar(&request, "r", "", "Request file to upload (default: stdin)")
	flag.StringVar(&response, "s", "", "Response file (default: stdout)")
	flag.StringVar(&engine, "e", "", "Engine to use")
	flag.StringVar(&config, "c", "client_config.json", "Config file to read (json)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: menagerie_cli [-v] [-r request file (json)] [-s response file (json)\n\tStandard input and output are used if the request/response are missing\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()

	if help || len(os.Args) == 1 {
		flag.Usage()
		os.Exit(0)
	}

	var requestBody []byte
	var err error

	if request != "" {
		requestBody, err = ioutil.ReadFile(request)
		glog.V(0).Infoln("using request file %s", request)
	} else { // use stdin
		requestBody, err = ioutil.ReadAll(os.Stdin)
		glog.V(0).Infoln("using stdin")
	}

	if err != nil {
		glog.Fatalf("Can't read request file %s.", request)
	}

	var file = os.Stdout
	if response != "" {
		file, err = os.Create(response)
		if err != nil {
			glog.Fatal("Can't create response file. ", err)
		}
		defer file.Close()
	} else {
		glog.Infoln("No output file given. Using stdout")
	}

	glog.V(0).Infoln("responseFile %#v", file)

	// this event triggers it all
	t := NewClient(config, engine, file)
	_ = t.doit(requestBody)
	os.Exit(0)
}
