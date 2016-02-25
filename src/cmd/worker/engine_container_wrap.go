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
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strconv"
	"time"

	"github.com/golang/glog"
)

var rand1 = rand.New(rand.NewSource(time.Now().UnixNano()))
var jobsdir = flag.String("jobsdir", "", "jobs dir in container")

type JobContext struct {
	dirname       string
	containerName string
}

type EngineContainerWrap struct {
	cfg.Engine
	index    int
	engineId string
	jobc     JobContext
}

func logName(tag string) string {
	t := time.Now()
	name := fmt.Sprintf("%s.log.%04d%02d%02d-%02d%02d%02d",
		tag,
		t.Year(),
		t.Month(),
		t.Day(),
		t.Hour(),
		t.Minute(),
		t.Second())
	return path.Join(flag.Lookup("log_dir").Value.String(), name)
}
func NewEngineContainerWrap(engineType string, index int) *EngineContainerWrap {
	e := cfg.GetEngine(engineType)
	if e == nil {
		return nil
	}
	return &EngineContainerWrap{
		Engine:   *e,
		index:    index,
		engineId: engineType + strconv.Itoa(index),
	}
}

func (ecwrap *EngineContainerWrap) OpenNewTask() (string, error) {
	name := ecwrap.engineId + "_" + strconv.Itoa(rand1.Intn(1000))
	ecwrap.jobc = JobContext{
		containerName: name,
		dirname:       path.Join(*jobsdir, name),
	}
	// build container mount
	if err := os.MkdirAll(ecwrap.jobc.dirname, 0700); err != nil {
		glog.Infoln("could not create dir worker for engine", ecwrap.engineId, ecwrap.jobc.containerName, ecwrap.jobc.dirname)
		return "", errors.New("error creating work dir")
	}

	docker_create_tokens := append(
		append([]string{"create", "-v", name + ":" + ecwrap.MountPoint, "-u", strconv.Itoa(ecwrap.User), "--name", ecwrap.jobc.containerName}, ecwrap.RunFlags...),
		[]string{ecwrap.Image, "/bin/bash", "-c", ecwrap.Cmd}...)
	docker_create_cmd := exec.Command("docker", docker_create_tokens...)
	docker_create_cmd.Stdout = os.Stdout
	docker_create_cmd.Stderr = os.Stderr
	glog.Infoln("volume create Command", docker_create_cmd.Args)
	err := docker_create_cmd.Run()
	if err != nil {
		return "", err
	}
	glog.Infoln("create dir worker for engine", ecwrap.engineId, ecwrap.jobc.dirname)
	return path.Join(ecwrap.jobc.dirname, ecwrap.InputFileName), nil
}

func (ecwrap *EngineContainerWrap) Stop() int {
	glog.Infoln("docker execution timeout", ecwrap.engineId)
	docker_kill := exec.Command("docker", "kill", ecwrap.jobc.containerName)
	docker_kill.Run()
	return 1
}

func (ecwrap *EngineContainerWrap) EngineId() string {
	return ecwrap.engineId
}

func (ecwrap *EngineContainerWrap) Cleanup() {
	rm_container_cmd := exec.Command("docker", "rm", "-v", ecwrap.jobc.containerName)
	glog.Infoln("rm container Command", rm_container_cmd.Args)
	rm_container_cmd.Run()
	rm_volume_cmd := exec.Command("docker", "volume", "rm", ecwrap.jobc.containerName)
	glog.Infoln("rm volume Command", rm_volume_cmd.Args)
	rm_volume_cmd.Run()
	glog.Infoln("Cleanup deletes", ecwrap.jobc.dirname, "for", ecwrap.engineId)
	os.RemoveAll(ecwrap.jobc.dirname)
}

// TODO: docker cp doesn't work correctly with user namespaces
// when it does then use it
func cpTo(src *os.File, destVol, destFile string) error {
	cmd := exec.Command(
		"docker", "run", "--rm", "-i", "-v", destVol+":/data",
		"busybox", "/bin/sh", "-c", "cat - > /data/"+destFile)
	cmd.Stdin = src
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cpFrom(vol, file string, dest *os.File) error {
	cmd := exec.Command(
		"docker", "run", "--rm", "-v", vol+":/data",
		"busybox", "cat", "/data/"+file)
	cmd.Stdout = dest
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// TODO better errors
func (ecwrap *EngineContainerWrap) Run() (string, error) {
	inputFile, err := os.Open(path.Join(ecwrap.jobc.dirname, ecwrap.InputFileName))
	if err != nil {
		return "", err
	}
	defer inputFile.Close()
	err = cpTo(inputFile,
		ecwrap.jobc.containerName, ecwrap.InputFileName)
	if err != nil {
		return "", err
	}
	docker_cmd := exec.Command("docker", "start", "-a", ecwrap.jobc.containerName)
	// TODO log stdout/stderr
	containerLogName := logName(ecwrap.jobc.containerName)
	contOut, oerr := os.Create(containerLogName)
	if oerr != nil {
		contOut = os.Stdout
	} else {
		defer func() {
			stat, serr := contOut.Stat()
			if serr == nil && stat.Size() == 0 {
				defer os.Remove(containerLogName)
			}
			contOut.Close()
		}()
	}
	docker_cmd.Stdout = contOut
	docker_cmd.Stderr = contOut
	glog.Infoln("create Command", docker_cmd.Args)
	err = docker_cmd.Run()
	if err != nil {
		glog.Errorln("error starting container ", err)
		return "", err
	}
	glog.Infoln("docker start finish ", ecwrap.engineId)

	resultFile, err := os.Create(path.Join(ecwrap.jobc.dirname, "result"))
	if err != nil {
		return "", err
	}
	defer resultFile.Close()
	err = cpFrom(ecwrap.jobc.containerName, "result", resultFile)
	if err != nil {
		glog.Error("Error copying result from container")
	}
	return path.Join(ecwrap.jobc.dirname, "result"), nil
}
