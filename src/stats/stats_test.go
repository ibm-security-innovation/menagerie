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
	"fmt"
	"testing"
)

func Test(t *testing.T) {
	s.prefix = "a"
	Inc("abc")
	Add("abc", 4)
	Set("123", 7)
	mule := String()
	var k1, k2 string
	var v1, v2 int
	var t1, t2 int
	fmt.Sscanf(mule, "%s %d %d\n%s %d %d", &k1, &v1, &t1, &k2, &v2, &t2)
	if !(k1 == "a.abc" && k2 == "a.123" && v1 == 5 && v2 == 7) &&
		!(k2 == "a.abc" && k1 == "a.123" && v2 == 5 && v1 == 7) {
		t.Error("unexpected mule syntax")
	}
	t.Log(k1, v1, k2, v2)
}
