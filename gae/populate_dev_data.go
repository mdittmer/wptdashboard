// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gae

import (
	"fmt"
	"log"
	"time"

	base "github.com/w3c/wptdashboard"
	"github.com/w3c/wptdashboard/metrics"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
)

func EnsureDevData(ctx context.Context) {
	tokens := []interface{}{&base.Token{}}
	urlFmtString := "http://localhost:8080/static/wptd/%s/%s"
	timeZero := time.Unix(0, 0)
	mkURL := func(hash string, summaryJsonGz string) string {
		return fmt.Sprintf(urlFmtString, hash, summaryJsonGz)
	}
	properTestRuns := []base.TestRun{
		base.TestRun{
			"chrome",
			"63.0",
			"linux",
			"3.16",
			"b952881825",
			mkURL("b952881825", "chrome-63.0-linux-summary.json.gz"),
			timeZero,
		},
		base.TestRun{
			"edge",
			"15",
			"windows",
			"10",
			"5d55258739",
			mkURL("5d55258739", "windows-10-sauce-summary.json.gz"),
			timeZero,
		},
		base.TestRun{
			"firefox",
			"57.0",
			"linux",
			"*",
			"fc70df1f75",
			mkURL("fc70df1f75", "firefox-57.0-linux-summary.json.gz"),
			timeZero,
		},
		base.TestRun{
			"safari",
			"10",
			"macos",
			"10.12",
			"fc2e57a502",
			mkURL("fc2e57a502", "safari-11.0-macos-10.12-sauce-summary.json.gz"),
			timeZero,
		},
	}
	testRuns := make([]interface{}, len(properTestRuns))
	for i, testRun := range properTestRuns {
		testRuns[i] = &testRun
	}
	metricsRuns := []interface{}{
		&metrics.MetricsRun{
			timeZero,
			timeZero,
			properTestRuns,
		},
	}

	tokenKindName := "Token"
	testRunKindName := "TestRun"
	metricsRunKindName := metrics.GetDatastoreKindName(metrics.MetricsRun{})

	ensureDataByCount(ctx, tokenKindName, tokens)
	ensureDataByCount(ctx, testRunKindName, testRuns)
	ensureDataByCount(ctx, metricsRunKindName, metricsRuns)
}

func ensureDataByCount(ctx context.Context, kindName string, data []interface{}) {
	log.Printf("Ensuring at least %d %s entities\n", len(data), kindName)
	count, err := datastore.NewQuery(kindName).Count(ctx)
	if err != nil {
		log.Fatalf("Failed to execute count query: %v", err)
	}
	if count >= len(data) {
		return
	}
	keys := make([]*datastore.Key, len(data))
	for i, _ := range data {
		keys[i] = datastore.NewIncompleteKey(ctx, kindName, nil)
	}
	datastore.PutMulti(ctx, keys, data)
}
