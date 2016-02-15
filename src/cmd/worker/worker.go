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
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"cfg"
	"stats"

	"github.com/golang/glog"
	"github.com/streadway/amqp"
)

var (
	engineType  = flag.String("engine", "", "engine name")
	workerIndex = flag.Int("i", 0, "index for tagging")
	joblimit    = flag.Int("joblimit", 0, "limit number of items to process (0 for no limit).")

	engine *EngineContainerWrap
)

func main() {
	cfg.Init()
	defer cfg.Finalize()

	engine = NewEngineContainerWrap(*engineType, *workerIndex)
	// TODO: pull image before reading from the queue
	conn, err := cfg.NewRabbitmqConn()
	if err != nil {
		glog.Errorln("Couldn't connect to rabbitmq", err)
		return
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		glog.Errorln("Couldn't open rabbitmq channel", err)
		return
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		*engineType, // name
		true,        // durable
		false,       // delete when unused
		false,       // exclusive
		false,       // no-wait
		nil,         // arguments
	)
	if err != nil {
		glog.Errorln("Couldn't open rabbitmq queue", *engineType, err)
		return
	}

	// make sure we fetch one at a time
	err = ch.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		glog.Errorln("Failed to set QoS to RmQ channel")
		return
	}

	consumer := fmt.Sprintf("pid-%d", os.Getpid())
	msgs, err := ch.Consume(
		q.Name,   // queue
		consumer, // consumer string
		false,    // auto-ack
		false,    // exclusive
		false,    // no-local
		false,    // no-wait
		nil,      // args
	)
	if err != nil {
		glog.Errorln("Failed to start RmQ consumption loop")
		return
	}

	for i := 0; i < *joblimit || *joblimit == 0; i++ {
		if d, gotMsg := <-msgs; gotMsg {
			work(d)
		} else {
			glog.Warningln("Message channel closed. Exiting.")
			break
		}
	}
	ch.Cancel(consumer, true)
}

func work(d amqp.Delivery) {
	job := string(d.Body)
	glog.Infoln("worker:work on job", job)
	if err := handleJob(job, d); err != nil {
		jobError(job, err.Error())
	}
	d.Ack(false) // we acknowledge whether the job succeeded or not - as we do not want it to stay in the queue
}

func handleJob(job string, d amqp.Delivery) error {
	stats.Inc(*engineType + ".job_started")
	stats.Inc("job_completion.started." + *engineType )
	path, err := engine.OpenNewTask()
	if err != nil {
		glog.Errorln("Error opening task", err)
		return errors.New("Error creating job")
	}
	defer engine.Cleanup()

	f, err := os.Create(path)
	if err != nil {
		glog.Errorln("Error creating file", err)
		return errors.New("Error creating job file")
	}
	defer f.Close()

	frontend := cfg.Frontend
	jobUrl := fmt.Sprintf("http://%s/files/%s/input", frontend, job)
	glog.Infoln("Getting job id", job, "from", jobUrl)
	res, err := http.Get(jobUrl)
	if err != nil {
		glog.Errorln("Error getting", jobUrl, err)
		return errors.New("Error generating job")
	}
	io.Copy(f, res.Body)
	f.Close()

	glog.Infoln("Running job", job)
	// TODO process file
	var resultFile string
	var resultError error
	done := make(chan struct{})
	go func() {
		resultFile, resultError = engine.Run()
		close(done)
	}()
	var timerc <-chan time.Time
	if engine.Timeout != 0 {
		timer := time.NewTimer(time.Duration(engine.Timeout) * time.Second)
		timerc = timer.C
		defer timer.Stop()
	}

	heartbeat := time.NewTicker(time.Second * 5)
	defer heartbeat.Stop()

	for {
		select {
		case <-done:
			if resultError != nil {
				glog.Errorln("Error running job", resultError)
			        stats.Inc("job_completion.error.running."+ *engineType)
			        stats.Inc(*engineType + ".error.running")
				return errors.New("Error running job")
			}
			glog.Infoln("Got result for job", job)
			result, err := os.Open(resultFile)
			if err != nil {
				glog.Errorln("Error opening result file", err)
			        stats.Inc("job_completion.error.result_processing."+ *engineType)
			        stats.Inc(*engineType + ".error.result_processing")
				return errors.New("Error processing result")
			}
			put, _ := http.NewRequest("PUT", fmt.Sprintf("http://%s/result/%s", frontend, job), result)
			_, err = http.DefaultClient.Do(put)
			if err != nil {
				glog.Errorln("Error sending results", err)
			        stats.Inc("job_completion.error.send_result."+ *engineType)
			        stats.Inc(*engineType + ".error.send_result")
				return errors.New("Error processing result")
			}
			glog.Infoln("Result sent to server for job", job)
			stats.Inc(*engineType + ".job_success")
			stats.Inc("job_completion.success."+ *engineType)
			return nil
		case <-timerc:
			stats.Inc(*engineType + ".job_timeout")
			stats.Inc("job_completion.timeout." +*engineType)
			glog.Errorln("Job", job, "timed out")
			engine.Stop()
			// TODO mark that we timed out
			return errors.New("Job timed out")

		case <-heartbeat.C:
			glog.Infoln("heartbeat for job", job)
		}
	}
}

func jobError(job string, msg string) {
	glog.Errorln("job", job, "terminated with error:", msg)

	body := strings.NewReader(msg)
	put, _ := http.NewRequest("PUT", fmt.Sprintf("http://%s/result/%s?status=error", cfg.Frontend, job), body)
	_, err := http.DefaultClient.Do(put)
	if err != nil {
		glog.Errorln("Error sending results", err)
		return
	}
}
