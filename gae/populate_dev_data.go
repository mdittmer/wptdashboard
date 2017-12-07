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
	timeZero := time.Unix(0, 0)

	// Follow pattern established in run/*.py data collection code.
	summaryUrlFmtString := "/static/wptd/%s/%s"
	mkSummaryUrl := func(hash string, summaryJsonGz string) string {
		return fmt.Sprintf(summaryUrlFmtString, hash, summaryJsonGz)
	}
	properTestRuns := []base.TestRun{
		base.TestRun{
			"chrome",
			"63.0",
			"linux",
			"3.16",
			"b952881825",
			mkSummaryUrl("b952881825", "chrome-63.0-linux-summary.json.gz"),
			timeZero,
		},
		base.TestRun{
			"edge",
			"15",
			"windows",
			"10",
			"5d55258739",
			mkSummaryUrl("5d55258739", "windows-10-sauce-summary.json.gz"),
			timeZero,
		},
		base.TestRun{
			"firefox",
			"57.0",
			"linux",
			"*",
			"fc70df1f75",
			mkSummaryUrl("fc70df1f75", "firefox-57.0-linux-summary.json.gz"),
			timeZero,
		},
		base.TestRun{
			"safari",
			"10",
			"macos",
			"10.12",
			"fc2e57a502",
			mkSummaryUrl("fc2e57a502", "safari-11.0-macos-10.12-sauce-summary.json.gz"),
			timeZero,
		},
	}
	// Follow pattern established in metrics/run/*.go data collection code.
	metricsUrlFmtString := fmt.Sprintf(
		"/static/wptd-metrics/%d-%d",
		timeZero.Unix(), timeZero.Unix()) +
		// Use unzipped JSON for local dev.
		"/%s.json"
	mkMetricsUrl := func(baseName string) string {
		return fmt.Sprintf(metricsUrlFmtString, baseName)
	}
	testRuns := make([]interface{}, len(properTestRuns))
	for i, testRun := range properTestRuns {
		testRuns[i] = &testRun
	}
	passRateMetadata := []interface{}{
		&metrics.PassRateMetadata{
			timeZero,
			timeZero,
			properTestRuns,
			mkMetricsUrl("pass-rates"),
		},
	}

	failuresMetadata := []interface{}{
		&metrics.FailuresMetadata{
			timeZero,
			timeZero,
			properTestRuns,
			mkMetricsUrl("chrome-failures"),
			"chrome",
		},
		&metrics.FailuresMetadata{
			timeZero,
			timeZero,
			properTestRuns,
			mkMetricsUrl("edge-failures"),
			"edge",
		},
		&metrics.FailuresMetadata{
			timeZero,
			timeZero,
			properTestRuns,
			mkMetricsUrl("firefox-failures"),
			"firefox",
		},
		&metrics.FailuresMetadata{
			timeZero,
			timeZero,
			properTestRuns,
			mkMetricsUrl("safari-failures"),
			"safari",
		},
	}

	tokenKindName := "Token"
	testRunKindName := "TestRun"
	passRateMetadataKindName := metrics.GetDatastoreKindName(
		metrics.PassRateMetadata{})
	failuresMetadataKindName := metrics.GetDatastoreKindName(
		metrics.FailuresMetadata{})

	ensureDataByCount(ctx, tokenKindName, tokens)
	ensureDataByCount(ctx, testRunKindName, testRuns)
	ensureDataByCount(ctx, passRateMetadataKindName, passRateMetadata)
	ensureDataByCount(ctx, failuresMetadataKindName, failuresMetadata)
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
