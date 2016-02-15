//
// Copyright 2015 IBM Corp. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
package main

import (
	"cfg"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/golang/glog"
)

var (
	workerExe = flag.String("workerexe", "worker", "path to the worker executable")

	wg      sync.WaitGroup
	sigchan []chan os.Signal
)

func workerSentry(engine cfg.Engine, index int) chan os.Signal {
	workersigchan := make(chan os.Signal, 1) //channel for signal delivery to worker processes
	engineType := engine.Name
	signalForStop := false
	go func() {
		defer func() {
			glog.Infoln("workerSentry out", engineType, index)
			wg.Done()
		}()
		for {

			glog.Infoln("workerSentry start", engineType, index, *workerExe)
			cmd := exec.Command(*workerExe,
				"-cfg", cfg.ConfigFile,
				"-engine-cfg", cfg.EngineConfigFile,
				"-i", fmt.Sprint(index),
				"-engine", engineType)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Start()
			starttime := time.Now()
			if err != nil {
				glog.Fatalln(err)
				return
			}
			glog.Infoln("Waiting for command to finish", engineType, index)
			c := make(chan string)
			go func() {
				cmd.Wait()
				glog.Infoln("Finished wait", engineType, index)
				close(c)
			}()
		outer:
			for {
				select {
				case res := <-c:
					//wait for container to finish
					glog.Infoln("finished worker execution", res, engineType, index)
					if signalForStop {
						return
					} else {
						if time.Since(starttime) < 30*time.Second {
							glog.Infoln("finished before sleep", engineType, index)
							glog.Flush()
							time.Sleep(30 * time.Second)
							glog.Infoln("finished sleep", engineType, index)
						}
						break outer
					}
				case sig := <-workersigchan:
					glog.Infoln("workersigchan signal ", engineType, index, sig)
					signalForStop = true
					cmd.Process.Signal(sig)
				}
			}

		}
	}()
	return workersigchan
}

func registerSignals(total_workers int) {
	signals := make(chan os.Signal, 1)
	sigchan = make([](chan os.Signal), total_workers, total_workers)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGHUP)
	go func() {
		defer glog.Infoln("signal listening ended")
		for {
			sig := <-signals
			switch sig {
			case syscall.SIGTERM, syscall.SIGKILL:
				for i := 0; i < cap(sigchan); i++ {
					sigchan[i] <- sig
				}
				return
			case syscall.SIGHUP:
				for i := 0; i < cap(sigchan); i++ {
					sigchan[i] <- sig
				}
			}
		}
	}()
}

func main() {
	cfg.Init()
	defer cfg.Finalize()

	n := 0
	for _, e := range cfg.Config.Engines {
		for i := 0; i < e.Workers; i++ {
			sigchan = append(sigchan, workerSentry(e, i))
			n++
		}
	}
	registerSignals(n)
	wg.Add(n)
	glog.Infoln("Waiting To Finish")
	wg.Wait()
	glog.Infoln("Gracefully terminated Program")
}
