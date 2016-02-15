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
package cfg

import (
	"fmt"
	"strings"
	"testing"

	"encoding/json"
)

func Test(t *testing.T) {
	data := `{"engines": [
	{ "name": "eaos32",
    "workers": 2,
    "image": "localhost:5000/eaos:1",
    "cmd": "/EAOS/run.sh params32.json",
    "mountpoint": "/var/data",
    "sizelimit": 1000000000,
    "inputfilename": "sample.zip",
    "user": 0,
    "timeout": 180 },
	{ "name": "eaos_w64",
    "workers": 2,
    "image": "localhost:5000/eaos:1",
    "cmd": "/EAOS/run.sh paramswow64.json",
    "mountpoint": "/var/data",
    "sizelimit": 1000000000,
    "inputfilename": "sample.zip",
    "user": 0,
    "timeout": 180 },
	{ "name": "pamcheck",
    "workers": 4,
    "image": "localhost:5000/pamcheck:1",
    "cmd": "/engine/run.sh",
    "mountpoint": "/var/data",
    "sizelimit": 1000000,
    "inputfilename": "sample.pcap",
    "timeout": 120,
    "user": 0 }
		]}
`
	var c Cfg
	data = strings.Replace(data, "\n", "", -1)
	fmt.Printf("%s\n", data)
	err := json.Unmarshal([]byte(data), &c)
	fmt.Printf("%+v\n", c)
	if err != nil || c.Engines[0].Name != "eaos32" || c.Engines[2].Workers != 4 {
		t.Error("Error unmarshalling json:", err)
	}
}
