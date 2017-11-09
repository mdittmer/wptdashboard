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

package wptdashboard

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
)

var httpGet = http.Get

type TestNames struct {
	testRun TestRun
	names   []string
}

func TestNamesFromTestRuns(testRuns []TestRun) (results []TestNames, err error) {
	// Encapsulate (TestNames, error) value for resultChan.
	type Result struct {
		testNames *TestNames
		err       error
	}

	// Sync on getting TestNames from all tests concurrently:
	// Channel for results, WaitGroup to wait for all results.
	resultChan := make(chan Result, len(testRuns))
	var wg sync.WaitGroup

	wg.Add(len(testRuns))
	fetch := func(testRun TestRun) {
		defer wg.Done()

		resp, err := httpGet(testRun.ResultsURL)
		if err != nil {
			resultChan <- Result{nil, err}
			return
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			resultChan <- Result{nil, err}
			return
		}

		result := make(map[string][]int32)
		err = json.Unmarshal(body, &result)
		if err != nil {
			resultChan <- Result{nil, err}
			return
		}

		testNames := make([]string, 0, len(result))
		for k := range result {
			testNames = append(testNames, k)
		}

		resultChan <- Result{&TestNames{testRun, testNames}, err}
	}

	for _, t := range testRuns {
		go fetch(t)
	}

	// Wait for results from all test runs.
	wg.Wait()
	close(resultChan)

	// Return all results, or first error encountered.
	for result := range resultChan {
		if result.err != nil {
			return nil, err
		}
		results = append(results, *result.testNames)
	}
	return results, nil
}
