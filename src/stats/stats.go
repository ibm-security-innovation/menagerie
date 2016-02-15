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
package stats

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"
)

var s = &stats{
	m: make(map[string]int),
}

type stats struct {
	mtx     sync.Mutex
	m       map[string]int
	prefix  string
	muleDir string
}

func init() {
	flag.StringVar(&s.prefix, "mule_prefix", "", "Prefix for mule stats")
	flag.StringVar(&s.muleDir, "mule_dir", "", "Location to store mule file. If empty will not record mule file.")

	go daemon()
}

func Inc(k string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.m[k]++
}

func Add(k string, n int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.m[k] += n
}

func Set(k string, n int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.m[k] = n
}

func Del(k string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	delete(s.m, k)
}

func Reset() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	reset()
}

func reset() {
	s.m = make(map[string]int)
}

func String() string {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return str()
}

func str() string {
	buf := bytes.NewBuffer(nil)
	now := time.Now().UTC().Unix()
	if s.prefix == "" {
		for k, v := range s.m {
			buf.Write([]byte(fmt.Sprintf("%s %d %d\n", k, v, now)))
		}
	} else {
		for k, v := range s.m {
			buf.Write([]byte(fmt.Sprintf("%s.%s %d %d\n", s.prefix, k, v, now)))
		}
	}

	return buf.String()
}

func strAndReset() string {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	res := str()
	reset()
	return res
}

func Flush() {
	// TODO check dir exists
	if s.muleDir == "" {
		return
	}

	str := strAndReset()
	if str == "" {
		return
	}

	exe := filepath.Base(os.Args[0])
	t := time.Now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("%s.%s.%d.mule", exe, t, os.Getpid())
	// TODO don't overwrite
	fpath := path.Join(s.muleDir, name)
	f, err := os.Create(fpath)
	if err != nil {
		// TODO log errors
	}
	defer f.Close()

	f.WriteString(str)
}

func daemon() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for _ = range ticker.C {
		Flush()
	}
}
